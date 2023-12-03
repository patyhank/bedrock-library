package bot

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	_ "github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl64"
	"math"
	"sync"
	_ "unsafe"
)

type World struct {
	chunks     map[world.ChunkPos]*Column
	r          cube.Range
	chunkMutex sync.Mutex
}

func NewWorld(r cube.Range) *World {
	return &World{
		chunks:     map[world.ChunkPos]*Column{},
		r:          r,
		chunkMutex: sync.Mutex{},
	}
}

var air, _ = chunk.StateToRuntimeID("minecraft:air", nil)
var airB, _ = world.BlockByRuntimeID(air)

// Block reads a block from the position passed. If a chunk is not yet loaded at that position, the chunk is
// loaded, or generated if it could not be found in the world save, and the block returned. Chunks will be
// loaded synchronously.
func (w *World) chunk(pos world.ChunkPos) *Column {
	w.chunkMutex.Lock()
	defer w.chunkMutex.Unlock()
	if c, ok := w.chunks[pos]; ok {
		return c
	}
	return nil
}
func (w *World) setChunk(pos world.ChunkPos, c *chunk.Chunk) {
	w.chunkMutex.Lock()
	defer w.chunkMutex.Unlock()
	w.chunks[pos] = newColumn(c)
}
func (w *World) Block(pos cube.Pos) world.Block {
	if w == nil || pos.OutOfBounds(w.r) {
		// Fast way out.
		return airB
	}

	c := w.chunk(chunkPosFromBlockPos(pos))
	c.Lock()
	defer c.Unlock()

	rid := c.Block(uint8(pos[0]), int16(pos[1]), uint8(pos[2]), 0)
	b, _ := world.BlockByRuntimeID(rid)
	return b
}

// Column represents the data of a chunk including the block entities and loaders. This data is protected
// by the mutex present in the chunk.Chunk held.
type Column struct {
	sync.Mutex

	*chunk.Chunk
}

// newColumn returns a new Column wrapper around the chunk.Chunk passed.
func newColumn(c *chunk.Chunk) *Column {
	return &Column{Chunk: c}
}

// chunkPosFromVec3 returns a chunk position from the Vec3 passed. The coordinates of the chunk position are
// those of the Vec3 divided by 16, then rounded down.
func chunkPosFromVec3(vec3 mgl64.Vec3) world.ChunkPos {
	return world.ChunkPos{
		int32(math.Floor(vec3[0])) >> 4,
		int32(math.Floor(vec3[2])) >> 4,
	}
}

// chunkPosFromBlockPos returns the ChunkPos of the chunk that a block at a cube.Pos is in.
func chunkPosFromBlockPos(p cube.Pos) world.ChunkPos {
	return world.ChunkPos{int32(p[0] >> 4), int32(p[2] >> 4)}
}

//go:linkname nbtBlocks github.com/df-mc/dragonfly/server/world.nbtBlocks
var nbtBlocks []bool

//go:linkname randomTickBlocks github.com/df-mc/dragonfly/server/world.randomTickBlocks
var randomTickBlocks []bool

//go:linkname liquidBlocks github.com/df-mc/dragonfly/server/world.liquidBlocks
var liquidBlocks []bool

//go:linkname blocks github.com/df-mc/dragonfly/server/world.blocks
var blocks []world.Block
