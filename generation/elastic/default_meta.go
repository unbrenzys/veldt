package elastic

import (
	"encoding/json"
	"fmt"

	"gopkg.in/olivere/elastic.v3"

	"github.com/unchartedsoftware/prism/binning"
	"github.com/unchartedsoftware/prism/generation/meta"
	jsonutil "github.com/unchartedsoftware/prism/util/json"
)

// PropertyMeta represents the meta data for a single property.
type PropertyMeta struct {
	Type    string           `json:"type"`
	Extrema *binning.Extrema `json:"extrema,omitempty"`
}

func isOrdinal(typ string) bool {
	return typ == "long" ||
		typ == "integer" ||
		typ == "short" ||
		typ == "byte" ||
		typ == "double" ||
		typ == "float" ||
		typ == "date"
}

func getPropertyMeta(client *elastic.Client, index string, field string, typ string) (*PropertyMeta, error) {
	p := PropertyMeta{
		Type: typ,
	}
	// if field is 'ordinal', get the extrema
	if isOrdinal(typ) {
		extrema, err := GetExtrema(client, index, field)
		if err != nil {
			return nil, err
		}
		p.Extrema = extrema
	}
	return &p, nil
}

func parsePropertiesRecursive(meta map[string]PropertyMeta, client *elastic.Client, index string, p map[string]interface{}, path string) error {
	children, ok := jsonutil.GetChildMap(p)
	if ok {
		for key, props := range children {
			subpath := key
			if path != "" {
				subpath = path + "." + key
			}
			subprops, hasProps := jsonutil.GetChild(props, "properties")
			if hasProps {
				// recurse further
				err := parsePropertiesRecursive(meta, client, index, subprops, subpath)
				if err != nil {
					return err
				}
			} else {
				typ, hasType := jsonutil.GetString(props, "type")
				// we don't support nested types
				if hasType && typ != "nested" {
					prop, err := getPropertyMeta(client, index, subpath, typ)
					if err != nil {
						return err
					}
					meta[subpath] = *prop

					// Parse out multi-field mapping
					fields, hasFields := jsonutil.GetChild(props, "fields")
					if hasFields {
						for fieldName := range fields {
							multiFieldPath := subpath + "." + fieldName
							prop, err = getPropertyMeta(client, index, multiFieldPath, typ)
							if err != nil {
								return err
							}
							meta[multiFieldPath] = *prop
						}
					}
				}
			}
		}
	}
	return nil
}

func parseProperties(client *elastic.Client, index string, props map[string]interface{}) (map[string]PropertyMeta, error) {
	// create empty map
	meta := make(map[string]PropertyMeta)
	err := parsePropertiesRecursive(meta, client, index, props, "")
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// DefaultMeta represents a meta data generator that produces default
// metadata with property types and extrema.
type DefaultMeta struct {
	MetaGenerator
}

// NewDefaultMeta instantiates and returns a pointer to a new generator.
func NewDefaultMeta(host string, port string) meta.GeneratorConstructor {
	return func(metaReq *meta.Request) (meta.Generator, error) {
		client, err := NewClient(host, port)
		if err != nil {
			return nil, err
		}
		m := &DefaultMeta{}
		m.host = host
		m.port = port
		m.req = metaReq
		m.client = client
		return m, nil
	}
}

// GetMeta returns the meta data for a given index.
func (g *DefaultMeta) GetMeta() ([]byte, error) {
	client := g.client
	metaReq := g.req
	// get the raw mappings
	mapping, err := GetMapping(client, metaReq.Index)
	if err != nil {
		return nil, err
	}
	// get nested 'properties' attribute of mappings payload
	// NOTE: If running a `mapping` query on an aliased index, the mapping
	// response will be nested under the original index name. Since we are only
	// getting the mapping of a single index at a time, we can simply get the
	// 'first' and only node.
	index, ok := jsonutil.GetRandomChild(mapping)
	if !ok {
		return nil, fmt.Errorf("Unable to retrieve the mappings response for %s",
			metaReq.Index)
	}
	// get mappings node
	mappings, ok := jsonutil.GetChildMap(index, "mappings")
	if !ok {
		return nil, fmt.Errorf("Unable to parse `mappings` from mappings response for %s",
			metaReq.Index)
	}
	// for each type, parse the mapping
	meta := make(map[string]interface{})
	for key, typ := range mappings {
		props, ok := jsonutil.GetChild(typ, "properties")
		if !ok {
			return nil, fmt.Errorf("Unable to parse `properties` from mappings response for type `%s` for %s",
				typ,
				metaReq.Index)
		}
		// parse json mappings into the property map
		typeMeta, err := parseProperties(client, metaReq.Index, props)
		if err != nil {
			return nil, err
		}
		meta[key] = typeMeta
	}
	// return
	return json.Marshal(meta)
}
