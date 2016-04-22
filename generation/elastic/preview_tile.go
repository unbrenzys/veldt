package elastic

import (
	"fmt"

	"gopkg.in/olivere/elastic.v3"

	"encoding/json"
	"github.com/unchartedsoftware/prism/generation/elastic/agg"
	"github.com/unchartedsoftware/prism/generation/elastic/param"
	"github.com/unchartedsoftware/prism/generation/elastic/query"
	"github.com/unchartedsoftware/prism/generation/tile"
)

// PreviewTile represents a tiling generator that produces an n x n tile containing
// preview data.
type PreviewTile struct {
	TileGenerator
	Binning *param.Binning
	Query   *query.Bool
	Metric  *agg.Metric
}

// NewPreviewTile instantiates and returns a pointer to a new generator.
func NewPreviewTile(host, port string) tile.GeneratorConstructor {
	return func(tileReq *tile.Request) (tile.Generator, error) {
		client, err := NewClient(host, port)
		if err != nil {
			return nil, err
		}
		binning, err := param.NewBinning(tileReq)
		if err != nil {
			return nil, err
		}
		query, err := query.NewBool(tileReq.Params)
		if err != nil {
			return nil, err
		}
		// optional
		metric, err := agg.NewMetric(tileReq.Params)
		if param.IsOptionalErr(err) {
			return nil, err
		}
		t := &PreviewTile{}
		t.Binning = binning
		t.Query = query
		t.Metric = metric
		t.req = tileReq
		t.host = host
		t.port = port
		t.client = client
		return t, nil
	}
}

// GetParams returns a slice of tiling parameters.
func (g *PreviewTile) GetParams() []tile.Param {
	return []tile.Param{
		g.Binning,
		g.Query,
		g.Metric,
	}
}

func (g *PreviewTile) getQuery() elastic.Query {
	return elastic.NewBoolQuery().
		Must(g.Binning.Tiling.GetXQuery()).
		Must(g.Binning.Tiling.GetYQuery()).
		Must(g.Query.GetQuery())
}

func (g *PreviewTile) getAgg() elastic.Aggregation {
	// create x aggregation
	xAgg := g.Binning.GetXAgg()
	// create y aggregation, add it as a sub-agg to xAgg
	yAgg := g.Binning.GetYAgg()
	xAgg.SubAggregation(yAggName, yAgg)

	// if there is a z field to sum, add sum agg to yAgg
	yAgg.SubAggregation(
		"tophits",
		elastic.NewTopHitsAggregation().
			Size(1).
			FetchSourceContext(
				elastic.NewFetchSourceContext(true).
					Include("text", "username", "timestamp")).
			Sort("timestamp", true))
	return xAgg
}

func (g *PreviewTile) parseResult(res *elastic.SearchResult) ([]byte, error) {
	binning := g.Binning
	// parse aggregations
	xAggRes, ok := res.Aggregations.Histogram(xAggName)
	if !ok {
		return nil, fmt.Errorf("Histogram aggregation '%s' was not found in response for request %s",
			xAggName,
			g.req.String())
	}

	// allocate bins buffer
	bins := make([]map[string]interface{}, binning.Resolution*binning.Resolution)

	// fill bins buffer
	for _, xBucket := range xAggRes.Buckets {
		x := xBucket.Key
		xBin := binning.GetXBin(x)
		yAggRes, ok := xBucket.Histogram(yAggName)
		if !ok {
			return nil, fmt.Errorf("Histogram aggregation '%s' was not found in response for request %s",
				yAggName,
				g.req.String())
		}
		for _, yBucket := range yAggRes.Buckets {
			y := yBucket.Key
			yBin := binning.GetYBin(y)
			index := xBin + binning.Resolution*yBin

			// extract top hits from each bucket
			var topHitsMap map[string]interface{}
			topHitsResult, ok := yBucket.TopHits("tophits")
			json.Unmarshal(*topHitsResult.Hits.Hits[0].Source, &topHitsMap)
			if !ok {
				return nil, fmt.Errorf("Top hits were not found in response for request %s", g.req.String())
			}

			// encode count
			bins[index] = topHitsMap
		}
	}
	return json.Marshal(bins)
}

// GetTile returns the marshalled tile data.
func (g *PreviewTile) GetTile() ([]byte, error) {
	// build query
	query := g.client.
		Search(g.req.Index).
		Size(1).
		Query(g.getQuery()).
		Aggregation("x", g.getAgg())
	// send query through equalizer
	res, err := query.Do()
	if err != nil {
		return nil, err
	}
	// parse and return results
	return g.parseResult(res)
}
