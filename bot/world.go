package bot

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	_ "github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/biome"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"golang.org/x/exp/maps"
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
	_ = biome.Ocean{}
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
func (w *World) setChunk(pos world.ChunkPos, c *chunk.Chunk, b map[cube.Pos]map[string]any) {
	w.chunkMutex.Lock()
	defer w.chunkMutex.Unlock()
	w.chunks[pos] = newColumn(c, b)
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
func (w *World) BlockEntity(pos cube.Pos) map[string]any {
	if w == nil || pos.OutOfBounds(w.r) {
		// Fast way out.
		return nil
	}

	c := w.chunk(chunkPosFromBlockPos(pos))
	c.Lock()
	defer c.Unlock()

	m := c.BlockEntities[pos]

	return m
}
func (w *World) SetBlockEntity(pos cube.Pos, data map[string]any) {
	if w == nil || pos.OutOfBounds(w.r) {
		// Fast way out.
		return
	}

	c := w.chunk(chunkPosFromBlockPos(pos))
	if c == nil {
		return
	}
	c.Lock()
	defer c.Unlock()

	m, ok := c.BlockEntities[pos]
	if !ok {
		c.BlockEntities[pos] = data
		return
	}
	maps.Copy(m, data)

	c.BlockEntities[pos] = m

	return
}
func (w *World) SetBlock(pos cube.Pos, b world.Block) world.Block {
	rid := world.BlockRuntimeID(b)
	bId := w.setBlock(pos, rid)
	id, _ := world.BlockByRuntimeID(bId)
	return id
}
func (w *World) setBlock(pos cube.Pos, rid uint32) uint32 {
	if w == nil || pos.OutOfBounds(w.r) {
		// Fast way out.
		return air
	}

	c := w.chunk(chunkPosFromBlockPos(pos))
	if c == nil {
		return air
	}
	//log.Info("locking")
	c.Lock()
	//log.Info("locked")
	defer c.Unlock()

	c.SetBlock(uint8(pos.X()), int16(pos.Y()), uint8(pos.Z()), 0, rid)
	return rid
}
func (w *World) Biome(pos cube.Pos) world.Biome {
	if w == nil || pos.OutOfBounds(w.r) {
		b, _ := world.BiomeByID(0)
		// Fast way out.
		return b
	}

	c := w.chunk(chunkPosFromBlockPos(pos))
	c.Lock()
	defer c.Unlock()

	id := int(c.Biome(uint8(pos[0]), int16(pos[1]), uint8(pos[2])))
	b, ok := world.BiomeByID(id)
	if !ok {
		b, _ := world.BiomeByID(0)
		// Fast way out.
		return b
	}
	return b
}

func (w *World) Range() cube.Range {
	return w.r
}

// Column represents the data of a chunk including the block entities and loaders. This data is protected
// by the mutex present in the chunk.Chunk held.
type Column struct {
	sync.Mutex

	*chunk.Chunk

	BlockEntities map[cube.Pos]map[string]any
}

// newColumn returns a new Column wrapper around the chunk.Chunk passed.
func newColumn(c *chunk.Chunk, b map[cube.Pos]map[string]any) *Column {
	return &Column{Chunk: c, BlockEntities: b}
}

// chunkPosFromVec3 returns a chunk position from the Vec3 passed. The coordinates of the chunk position are
// those of the Vec3 divided by 16, then rounded down.
func chunkPosFromVec3(vec3 mgl64.Vec3) world.ChunkPos {
	return world.ChunkPos{
		int32(math.Floor(vec3[0])) >> 4,
		int32(math.Floor(vec3[2])) >> 4,
	}
}

// vec32To64 converts a mgl32.Vec3 to a mgl64.Vec3.
func vec32To64(vec3 mgl32.Vec3) mgl64.Vec3 {
	return mgl64.Vec3{float64(vec3[0]), float64(vec3[1]), float64(vec3[2])}
}

// vec64To32 converts a mgl64.Vec3 to a mgl32.Vec3.
func vec64To32(vec3 mgl64.Vec3) mgl32.Vec3 {
	return mgl32.Vec3{float32(vec3[0]), float32(vec3[1]), float32(vec3[2])}
}

// blockPosFromProtocol ...
func blockPosFromProtocol(pos protocol.BlockPos) cube.Pos {
	return cube.Pos{int(pos.X()), int(pos.Y()), int(pos.Z())}
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
