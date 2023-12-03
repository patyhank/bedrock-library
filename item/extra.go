package item

// EmptyMap empty map
type EmptyMap struct{}

// EncodeItem ...
func (e EmptyMap) EncodeItem() (name string, meta int16) {
	return "minecraft:empty_map", 0
}
