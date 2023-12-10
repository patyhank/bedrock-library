package main

import (
	"bytes"
	_ "embed"
	"fmt"
	generateutils "git.patyhank.net/falloutBot/bedrocklib/cmd/generatorutils"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"os"
	"sort"
	"strings"
	"text/template"
	"unsafe"
)

//go:embed blocks.go.tmpl
var tempSource string

var temp = template.Must(template.
	New("block_template").
	Funcs(template.FuncMap{
		"UpperTheFirst": generateutils.UpperTheFirst,
		"ToGoTypeName":  generateutils.ToGoTypeName,
		"Generator":     func() string { return "generator/blocks/main.go" },
	}).
	Parse(tempSource),
)

// blockState holds a combination of a name and properties, together with a version.
type blockState struct {
	Name       string         `nbt:"name"`
	Properties map[string]any `nbt:"states"`
	Version    int32          `nbt:"version"`
}
type State struct {
	//blockState
	//Name       string
	//Properties string
	//HasItem    bool
	//HasBlock   bool
	Name string
	//ItemRID int32
}

func main() {
	genFile()
}

//var (
//	itemRuntimeIDsToNames = map[int32]string{}
//	// itemNamesToRuntimeIDs holds a map to translate item string IDs to runtime IDs.
//	itemNamesToRuntimeIDs = map[string]int32{}
//)

func genFile() {
	_ = item.Arrow{}
	_ = block.Air{}
	_ = world.New()
	itemRuntimeIDData, _ := os.ReadFile("item_runtime_ids.nbt")

	var ss []State
	var m map[string]int32
	err := nbt.Unmarshal(itemRuntimeIDData, &m)
	if err != nil {
		panic(err)
	}
	for name, rid := range m {
		_, b := world.ItemByName(name, int16(rid))
		if !b {
			ss = append(ss, State{
				Name: name,
			})
		}
	}

	var source bytes.Buffer
	temp.Execute(&source, ss)

	os.WriteFile("../../../extra/extras.go", source.Bytes(), 0o666)
}

// hashProperties produces a hash for the block properties held by the blockState.
func hashProperties(properties map[string]any) string {
	if properties == nil {
		return ""
	}
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	var b strings.Builder
	for _, k := range keys {
		switch v := properties[k].(type) {
		case bool:
			if v {
				b.WriteByte(1)
			} else {
				b.WriteByte(0)
			}
		case uint8:
			b.WriteByte(v)
		case int32:
			a := *(*[4]byte)(unsafe.Pointer(&v))
			b.Write(a[:])
		case string:
			b.WriteString(v)
		default:
			// If block encoding is broken, we want to find out as soon as possible. This saves a lot of time
			// debugging in-game.
			panic(fmt.Sprintf("invalid block property type %T for property %v", v, k))
		}
	}

	return b.String()
}
