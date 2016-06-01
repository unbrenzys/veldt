package agg

import (
	"fmt"

	"gopkg.in/olivere/elastic.v3"

	"github.com/unchartedsoftware/prism/generation/elastic/param"
	"github.com/unchartedsoftware/prism/util/json"
)

const (
	defaultTermsSize = 10
)

// TopTerms represents params for extracting particular topics.
type TopTerms struct {
	Field string
	Size  uint32
}

// NewTopTerms instantiates and returns a new topic parameter object.
func NewTopTerms(params map[string]interface{}) (*TopTerms, error) {
	params, ok := json.GetChild(params, "top_terms")
	if !ok {
		return nil, fmt.Errorf("%s `top_terms` aggregation parameter", param.MissingPrefix)
	}
	field, ok := json.GetString(params, "field")
	if !ok {
		return nil, fmt.Errorf("TopTerms `field` parameter missing from tiling param %v", params)
	}
	return &TopTerms{
		Field: field,
		Size:  uint32(json.GetNumberDefault(params, defaultTermsSize, "size")),
	}, nil
}

// GetHash returns a string hash of the parameter state.
func (p *TopTerms) GetHash() string {
	return fmt.Sprintf("%s:%d", p.Field, p.Size)
}

// GetAgg returns an elastic aggregation.
func (p *TopTerms) GetAgg() *elastic.TermsAggregation {
	return elastic.NewTermsAggregation().
		Field(p.Field).
		Size(int(p.Size))
}