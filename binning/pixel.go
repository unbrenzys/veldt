package binning

import (
	"math"

	"github.com/unchartedsoftware/prism/util"
)

// PixelBounds represents a bounding box in pixel coordinates.
type PixelBounds struct {
	TopLeft     *PixelCoord
	BottomRight *PixelCoord
}

// PixelCoord represents a point in pixel coordinates.
type PixelCoord struct {
	X uint64 `json:"x"`
	Y uint64 `json:"y"`
}

// MaxLevelSupported represents the maximum zoom level supported by the pixel coordinate system.
const MaxLevelSupported = uint64(24)

// MaxTileResolution represents the maximum bin resolution of a tile
const MaxTileResolution = uint64(256)

// number of pixels across the x / y dimensions at maximum zoom level
var maxPixels = float64(MaxTileResolution) * math.Pow(2, float64(MaxLevelSupported))

// LonLatToPixelCoord translates a geographic coordinate to a pixel coordinate.
func LonLatToPixelCoord(lonLat *LonLat) *PixelCoord {
	// Converting to range from [0:1] where 0,0 is top left
	normalizedTile := LonLatToFractionalTile(lonLat, 0)
	normalizedCoord := &Coord{
		X: normalizedTile.X,
		Y: normalizedTile.Y,
	}
	return &PixelCoord{
		X: uint64(math.Floor(normalizedCoord.X * maxPixels)),
		Y: uint64(math.Floor(normalizedCoord.Y * maxPixels)),
	}
}

// CoordToPixelCoord translates a coordinate to a pixel coordinate.
func CoordToPixelCoord(coord *Coord, bounds *Bounds) *PixelCoord {
	// Converting to range from [0:1] where 0,0 is top left
	normalizedTile := CoordToFractionalTile(coord, 0, bounds)
	normalizedCoord := &Coord{
		X: normalizedTile.X,
		Y: normalizedTile.Y,
	}
	return &PixelCoord{
		X: uint64(math.Floor(normalizedCoord.X * maxPixels)),
		Y: uint64(math.Floor(normalizedCoord.Y * maxPixels)),
	}
}

// GetTilePixelBounds returns the pixel coordniate bounds of the tile coordinate.
func GetTilePixelBounds(tile *TileCoord) *PixelBounds {
	pow2 := math.Pow(2, float64(tile.Z))
	// Converting to range from [0:1] where 0,0 is top left
	xMin := float64(tile.X) / pow2
	xMax := float64(tile.X+1) / pow2
	yMin := float64(tile.Y) / pow2
	yMax := float64(tile.Y+1) / pow2
	return &PixelBounds{
		TopLeft: &PixelCoord{
			X: uint64(util.Round(xMin * maxPixels)),
			Y: uint64(util.Round(yMin * maxPixels)),
		},
		BottomRight: &PixelCoord{
			X: uint64(util.Round(xMax * maxPixels)),
			Y: uint64(util.Round(yMax * maxPixels)),
		},
	}
}
