package main

import (
	"encoding/json"
	"os"

	"github.com/sandertv/gophertunnel/minecraft/nbt"
)

type T struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

var tt []T

func main() {
	file, _ := os.ReadFile("runtime_item_states.1_21_93.json") // 40 TBD
	json.Unmarshal(file, &tt)
	m := map[string]int32{}
	for _, t := range tt {
		m[t.Name] = int32(t.Id)
	}
	marshal, _ := nbt.Marshal(m)
	_ = os.WriteFile("item_runtime_ids.nbt", marshal, 0644)
}
