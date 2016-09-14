package s3

import (
	"fmt"
	"github.com/unchartedsoftware/prism/generation/tile"
)

// TileGenerator represents a base generator that uses elasticsearch for its
// backend.
type TileGenerator struct {
	baseURL string
	req     *tile.Request
}

// GetHash returns the hash for this generator.
func (g *TileGenerator) GetHash() string {
	return fmt.Sprintf("%s", g.baseURL)
}