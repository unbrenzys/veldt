package binning

import (
	"math"

	"github.com/unchartedsoftware/veldt/geometry"
)

const (
	// MaxLevelSupported represents the maximum zoom level supported by the pixel coordinate system.
	MaxLevelSupported = float64(24)
	// MaxTileResolution represents the maximum bin resolution of a tile
	MaxTileResolution = float64(256)
	// MaxPixels represents the maximum value of the pixel coordinates
	MaxPixels = MaxTileResolution * (1 << uint64(MaxLevelSupported))
)

// PixelCoord represents a point in pixel coordinates where 0,0 is BOTTOM-LEFT.
type PixelCoord struct {
	X uint64 `json:"x"`
	Y uint64 `json:"y"`
}

// NewPixelCoord instantiates and returns a pointer to a PixelCoord.
func NewPixelCoord(x, y uint64) *PixelCoord {
	return &PixelCoord{
		X: uint64(math.Max(0, math.Min(float64(MaxPixels-1), float64(x)))),
		Y: uint64(math.Max(0, math.Min(float64(MaxPixels-1), float64(y)))),
	}
}

// LonLatToPixelCoord translates a geographic coordinate to a pixel coordinate.
func LonLatToPixelCoord(lonLat *LonLat) *PixelCoord {
	// Converting to range from [0:1] where 0,0 is bottom-left
	normalized := LonLatToFractionalTile(lonLat, 0)
	return NewPixelCoord(
		uint64(normalized.X*MaxPixels),
		uint64(normalized.Y*MaxPixels))
}

// CoordToPixelCoord translates a coordinate to a pixel coordinate.
func CoordToPixelCoord(coord *geometry.Coord, bounds *geometry.Bounds) *PixelCoord {
	// Converting to range from [0:1] where 0,0 is bottom-left
	normalized := CoordToFractionalTile(coord, 0, bounds)
	return NewPixelCoord(
		uint64(normalized.X*MaxPixels),
		uint64(normalized.Y*MaxPixels))
}
