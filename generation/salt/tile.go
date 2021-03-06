package salt

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"

	"github.com/liyinhgqw/typesafe-config/parse"

	"github.com/unchartedsoftware/veldt"
	"github.com/unchartedsoftware/veldt/binning"
	"github.com/unchartedsoftware/veldt/generation/batch"
	"github.com/unchartedsoftware/veldt/util/json"
)

// This file contains the basic functionality of all salt tiles
//
// TileData is the main class herein, and is a the data needed by Salt to
// produce a tile

// We fake true class functionality by putting three function members in
// TileData.  The following three function types define and describe those
// three implementation-specific functions

// ConfigBuilder is function responsible for producing Salt tile
// configurations
type ConfigBuilder func() (map[string]interface{}, error)

// TileConverter converts the output of the Salt tile server into the format
// Veldt wants for a given tile type
type TileConverter func(*binning.TileCoord, []byte) ([]byte, error)

// DefaultTileConstructor constructs a tile when salt doesn't return one
type DefaultTileConstructor func() ([]byte, error)

// TileData represents the data needed by every tile request that is backed by
// Salt to connect to the Salt tile server
type TileData struct {
	tileType string
	// The configuration defining how we connect to the RabbitMQ server
	rmqConfig *Config
	// The parameters passed into the Parse method
	// We just keep them around until later, so that our single-tile and batch-
	// tile code can work the same way.
	parameters *map[string]interface{}
	// A builder that can build Salt configurations for us
	buildConfig ConfigBuilder
	// A converter that can convert the results from what Salt gives us to
	// what Veldt wants
	convert TileConverter
	// A function to construct a default tile, for when Salt tells us there
	// is no information in the requested tile
	buildDefault DefaultTileConstructor
}

type tileResult struct {
	coord *binning.TileCoord
	data  []byte
}

// datasets contains a mapping listing the configurations of all known
// datasets, indexed by dataset name.
var datasets = make(map[string]string)

// getDatasetName gets the name of a dataset from its raw configuration
func getDatasetName(datasetConfigRaw []byte) (string, error) {
	datasetConfigMap, err := parse.Parse("dataset", string(datasetConfigRaw))
	if err != nil {
		return "", err
	}
	datasetConfig := datasetConfigMap.GetConfig()
	result, err := datasetConfig.GetString("name")
	if err != nil {
		return "", err
	}
	return stripTerminalQuotes(result), nil
}

// setupConnection is used by every Salt tile request to initialize the
// connection with the salt server, and to initialize the dataset this request
// requires
func setupConnection(rmqConfig *Config, datasetConfigs ...[]byte) {
	// Send any dataset configurations to salt immediately
	// Need a connection for that
	connection, err := NewConnection(rmqConfig)
	if err != nil {
		Errorf("Error connecting to salt server to configure datasets: %v", err)
	} else {
		for _, datasetConfig := range datasetConfigs {
			name, err := getDatasetName(datasetConfig)
			if err != nil {
				Errorf("Error registering dataset: can't find name of dataset %v", string(datasetConfig))
			} else {
				_, err = connection.Dataset(datasetConfig)
				if err != nil {
					Errorf("Error registering dataset %v: %v", name, err)
				} else {
					Infof("Registering dataset %s", name)
					datasets[name] = string(datasetConfig)
				}
			}
		}
	}
}

// Parse parses the parameters for a heatmap tile
func (t *TileData) Parse(params map[string]interface{}) error {
	t.parameters = &params
	return nil
}

// Create generates a single tile from the provided URI, tile coordinate, and
// query parameters.  It does this by wrapping the information as a multi-tile
// request with a single tile in it, and calling CreateTiles.
func (t *TileData) Create(uri string, coord *binning.TileCoord, query veldt.Query) ([]byte, error) {
	responseChan := make(chan batch.TileResponse, 1)
	request := &batch.TileRequest{
		Params:        *t.parameters,
		URI:           uri,
		Coord:         coord,
		Query:         query,
		ResultChannel: responseChan,
	}
	t.CreateTiles([]*batch.TileRequest{request})
	response := <-responseChan
	if response.Tile != nil {
		Debugf("Create: Got response tile of length %d", len(response.Tile))
	} else {
		Debugf("Create: Got nil response tile")
	}
	if response.Err != nil {
		Debugf("Create: Got non-nil error")
	} else {
		Debugf("Create: no error")
	}
	return response.Tile, response.Err
}

// CreateTiles generates multiple tiles from the provided information
func (t *TileData) CreateTiles(requests []*batch.TileRequest) {
	Infof("CreateTiles: Processing %d requests of type %s", len(requests), t.tileType)
	// Create our connection
	connection, err := NewConnection(t.rmqConfig)
	if err != nil {
		for _, request := range requests {
			request.ResultChannel <- batch.TileResponse{
				Tile: nil,
				Err:  err,
			}
		}
		return
	}

	// Go through every request, generating the joint (non-tile-coordinate)
	// configuration for each.
	//
	// Requests with the same joint configuration are consolidated.
	consolidatedRequests := make([]*jointRequest, 0)
	for _, tileRequest := range requests {
		request, err := t.extractJointRequest(tileRequest)
		if err != nil {
			tileRequest.ResultChannel <- batch.TileResponse{
				Tile: nil,
				Err:  err,
			}
		} else {
			requestMerged := false
			for _, currentRequest := range consolidatedRequests {
				if !requestMerged && canMerge(request, currentRequest) {
					currentRequest.merge(request)
					requestMerged = true
				}
			}
			if !requestMerged {
				consolidatedRequests = append(consolidatedRequests, request)
			}
		}
	}

	// Requests are all merged
	// Now actually make our requests of the server
	for _, request := range consolidatedRequests {
		Infof("Request for %d tiles for dataset %s of type %s", len(request.tiles), request.dataset, t.tileType)
		// Make sure relevant parameters are available to conversion functions
		err = t.Parse(request.params)
		if nil != err {
			for _, tileReq := range request.tiles {
				tileReq.sendTo <- batch.TileResponse{
					Tile: nil,
					Err:  err,
				}
			}
		} else {
			// Create our consolidated configuration
			fullConfig := make(map[string]interface{})
			fullConfig["tile"] = request.tileConfig
			fullConfig["query"] = request.query
			fullConfig["dataset"] = datasets[request.dataset]
			// Put in all our tile requests, recording our response channel for each as we go
			responseChannels := make(map[string]chan batch.TileResponse)
			tileSpecs := make([]interface{}, 0)
			for _, tileReq := range request.tiles {
				c := tileReq.coord
				tileSpec := make(map[string]interface{})
				tileSpec["level"] = int(c.Z)
				tileSpec["x"] = int(c.X)
				tileSpec["y"] = int(c.Y)
				tileSpecs = append(tileSpecs, tileSpec)
				responseChannels[coordToString(int(c.Z), int(c.X), int(c.Y))] = tileReq.sendTo
			}
			fullConfig["tile-specs"] = tileSpecs

			// Marshal the consolidated request into a string
			requestBytes, err := json.Marshal(fullConfig)
			if err != nil {
				for _, channel := range responseChannels {
					channel <- batch.TileResponse{
						Tile: nil,
						Err:  err,
					}
				}
			} else {
				// Send the marshalled request to Salt, and await a response
				result, err := connection.QueryTiles(requestBytes)
				if err != nil {
					for _, channel := range responseChannels {
						channel <- batch.TileResponse{
							Tile: nil,
							Err:  err,
						}
					}
				} else {
					// Unpack the results
					tiles := unpackTiles(result)
					for key, channel := range responseChannels {
						tile, ok := tiles[key]
						if ok {
							Debugf("Found tile for key %s[%s] of length %d", key, t.tileType, len(tile.data))
							converted, err := t.convert(tile.coord, tile.data)
							Debugf("Converted tile for key %s[%s] had length %d", key, t.tileType, len(converted))
							channel <- batch.TileResponse{
								Tile: converted,
								Err:  err,
							}
						} else {
							// No tile, but no error either
							Debugf("No tile found for key %s", key)
							defaultTile, err := t.buildDefault()
							channel <- batch.TileResponse{
								Tile: defaultTile,
								Err:  err,
							}
						}
					}
				}
			}
		}
	}
}

// separateTileRequest contains the portion of a tile request that is specific
// to a single tile
type separateTileRequest struct {
	coord  *binning.TileCoord
	sendTo chan batch.TileResponse
}

// jointRequest consolidates several joinable tile requests, keeping the
// information they have in common separate from the information that is
// specific to each
type jointRequest struct {
	tileConfig map[string]interface{}
	query      map[string]interface{}
	params     map[string]interface{}
	dataset    string
	tiles      []*separateTileRequest
}

// canMerge indicates whether two consolidated requests can be merged together
func canMerge(a, b *jointRequest) bool {
	if !reflect.DeepEqual(a.tileConfig, b.tileConfig) {
		return false
	}
	if !reflect.DeepEqual(a.query, b.query) {
		return false
	}
	return a.dataset == b.dataset
}

// merge merges two consolidated requests (which must previously have been
// determined to be mergable using canMerge)
func (j *jointRequest) merge(from *jointRequest) {
	for _, tile := range from.tiles {
		j.tiles = append(j.tiles, tile)
	}
}

// extractJointRequest takes a single batched tile request, and converts it
// into a joinable consolidated request
func (t *TileData) extractJointRequest(request *batch.TileRequest) (*jointRequest, error) {
	err := t.Parse(request.Params)
	if nil != err {
		return nil, err
	}
	tileConfig, err := t.buildConfig()
	if err != nil {
		return nil, err
	}

	var queryConfig map[string]interface{}
	if request.Query != nil {
		saltQuery, ok := request.Query.(Query)
		if !ok {
			return nil, fmt.Errorf("query for salt tile was not a salt query")
		}

		var err error
		queryConfig, err = saltQuery.Get()
		if err != nil {
			return nil, err
		}
	}

	separateRequest := separateTileRequest{request.Coord, request.ResultChannel}
	separateRequests := []*separateTileRequest{&separateRequest}

	return &jointRequest{tileConfig, queryConfig, request.Params, request.URI, separateRequests}, nil
}

// Get a unique string ID for use in maps for a tile coordinate
func coordToString(level, x, y int) string {
	max := 1 << uint64(level)
	digits := int64(math.Floor(math.Log10(float64(max)))) + 1
	format := fmt.Sprintf("%%02d:%%0%dd:%%0%dd", digits, digits)
	return fmt.Sprintf(format, level, x, y)
}

// unpackTiles unpacks the message sent to us by salt into a series of tiles,
// keyed by the coordToString function above
func unpackTiles(saltMsg []byte) map[string]tileResult {
	p := 0
	maxP := len(saltMsg)
	results := make(map[string]tileResult)
	for p < maxP {
		level := binary.BigEndian.Uint64(saltMsg[p : p+8])
		p = p + 8
		x := binary.BigEndian.Uint64(saltMsg[p : p+8])
		p = p + 8
		y := binary.BigEndian.Uint64(saltMsg[p : p+8])
		p = p + 8
		size := int(binary.BigEndian.Uint64(saltMsg[p : p+8]))
		p = p + 8
		key := coordToString(int(level), int(x), int(y))
		coord := &binning.TileCoord{X: uint32(x), Y: uint32(y), Z: uint32(level)}
		Debugf("Unpacking tile [%d: %d, %d] = %s, length = %d", level, x, y, key, size)
		results[key] = tileResult{coord, saltMsg[p : p+size]}
		p = p + size
	}
	return results
}
