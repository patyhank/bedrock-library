package extra

import (
	"fmt"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/model"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/world"
)

const (
	hashShulkerBox = iota + 500
	hashHopper
)

// ShulkerBox is a container block which may be used to store items. Chests may also be paired to create a bigger
// single container.
// The empty value of ShulkerBox is not valid. It must be created using block.NewChest().
type ShulkerBox struct {
	Colour item.Colour
	Facing cube.Direction
}

// EncodeItem ...
func (c ShulkerBox) EncodeItem() (name string, meta int16) {
	return fmt.Sprintf("minecraft:%s_shulker_box", c.Colour.String()), 0
}

// EncodeBlock ...
func (c ShulkerBox) EncodeBlock() (name string, properties map[string]any) {
	return fmt.Sprintf("minecraft:%s_shulker_box", c.Colour.String()), nil
}
func (c ShulkerBox) Hash() uint64 {
	return hashShulkerBox | uint64(c.Colour.Uint8())<<8
}

func (c ShulkerBox) Model() world.BlockModel {
	return model.Solid{}
}

// EmptyMap empty map
type EmptyMap struct{}

// EncodeItem ...
func (e EmptyMap) EncodeItem() (name string, meta int16) {
	return "minecraft:empty_map", 0
}

// Hopper is Hopper
type Hopper struct {
	Facing cube.Direction
}

// EncodeItem ...
func (c Hopper) EncodeItem() (name string, meta int16) {
	return fmt.Sprintf("minecraft:hopper"), 0
}

// EncodeBlock ...
func (c Hopper) EncodeBlock() (name string, properties map[string]any) {
	return fmt.Sprintf("minecraft:hopper"), nil
}
func (c Hopper) Hash() uint64 {
	return hashHopper
}

func (c Hopper) Model() world.BlockModel {
	return model.Solid{}
}
