package elastic

import (
	"encoding/json"
	"fmt"

	"gopkg.in/olivere/elastic.v3"

	"github.com/unchartedsoftware/prism/generation/elastic/agg"
	"github.com/unchartedsoftware/prism/generation/elastic/param"
	"github.com/unchartedsoftware/prism/generation/elastic/query"
	"github.com/unchartedsoftware/prism/generation/tile"
)

// MicroTile represents a tiling generator that produces a tile.
type MicroTile struct {
	TileGenerator
	Binning    *param.Binning
	Query      *query.Bool
	MacroMicro *param.MacroMicro
	TopHits    *agg.TopHits
}

// NewMicroTile instantiates and returns a pointer to a new generator.
func NewMicroTile(host, port string) tile.GeneratorConstructor {
	return func(tileReq *tile.Request) (tile.Generator, error) {
		client, err := NewClient(host, port)
		if err != nil {
			return nil, err
		}
		elastic, err := param.NewElastic(tileReq)
		if err != nil {
			return nil, err
		}
		binning, err := param.NewBinning(tileReq)
		if err != nil {
			return nil, err
		}
		macromicro, err := param.NewMacroMicro(tileReq)
		if err != nil {
			return nil, err
		}
		query, err := query.NewBool(tileReq.Params)
		if err != nil {
			return nil, err
		}
		topHits, err := agg.NewTopHits(tileReq.Params)
		if err != nil {
			return nil, err
		}
		t := &MicroTile{}
		t.Elastic = elastic
		t.Binning = binning
		t.MacroMicro = macromicro
		t.Query = query
		t.TopHits = topHits
		t.req = tileReq
		t.host = host
		t.port = port
		t.client = client
		return t, nil
	}
}

// GetParams returns a slice of tiling parameters.
func (g *MicroTile) GetParams() []tile.Param {
	return []tile.Param{
		g.Binning,
		g.MacroMicro,
		g.Query,
		g.TopHits,
	}
}

func (g *MicroTile) getQuery() elastic.Query {
	return elastic.NewBoolQuery().
		Must(g.Binning.Tiling.GetXQuery()).
		Must(g.Binning.Tiling.GetYQuery()).
		Must(g.Query.GetQuery())
}

func (g *MicroTile) getAgg() elastic.Aggregation {
	return g.TopHits.GetAgg()
}

func (g *MicroTile) parseResult(res *elastic.SearchResult) ([]byte, error) {
	// parse aggregations
	topHitsAgg, ok := res.Aggregations.TopHits(topHitsAggName)
	if !ok {
		return nil, fmt.Errorf("Top hits were not found in response for request %s", g.req.String())
	}
	// loop over raw hit results for the bin and unmarshall them into a list
	topHits := make([]map[string]interface{}, len(topHitsAgg.Hits.Hits))
	for index, hit := range topHitsAgg.Hits.Hits {
		src := make(map[string]interface{})
		err := json.Unmarshal(*hit.Source, &src)
		if err != nil {
			return nil, fmt.Errorf("Top hits could not be unmarshalled from response for request %s",
				g.req.String())
		}
		topHits[index] = src
	}
	return json.Marshal(topHits)
}

// GetTile returns the marshalled tile data.
func (g *MicroTile) GetTile() ([]byte, error) {
	// first pass to get the count for the tile
	res, err := g.Elastic.GetSearchService(g.client).
		Index(g.req.Index).
		Size(0).
		Query(g.getQuery()).
		Do()
	if err != nil {
		return nil, err
	}
	if res.Hits.TotalHits <= g.MacroMicro.Threshold {
		// generate micro tile
		query := g.Elastic.GetSearchService(g.client).
			Index(g.req.Index).
			Size(0).
			Query(g.getQuery()).
			Aggregation(topHitsAggName, g.getAgg())
		// send query through equalizer
		res, err = query.Do()
		if err != nil {
			return nil, err
		}
		// parse and return results
		return g.parseResult(res)
	}
	// above threshold, return nil
	return nil, nil
}