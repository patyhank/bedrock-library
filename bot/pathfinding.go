package bot

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/model"
	"github.com/df-mc/dragonfly/server/world"
)

type Finder[Node cube.Pos] struct {
	w           *world.World
	c           *Client
	allowFlight bool
}

func (f Finder[_]) AllowStanding(n cube.Pos) bool {
	b := f.w.Block(n)
	b2 := f.w.Block(n.Side(cube.FaceUp))
	//b2 := f.w.Block(n.Side(cube.FaceUp))
	//b2, err := f.w.GetBlock(n.Offset(0, 1, 0))
	//if err.Err != basic.NoError {
	//	return false
	//}
	b3 := f.w.Block(n.Side(cube.FaceDown))
	//b3, err := f.w.GetBlock(n.Offset(0, -1, 0))
	//if err.Err != basic.NoError {
	//	return false
	//}

	if !f.allowFlight {
		if _, ok := b3.Model().(model.Empty); ok {
			return false
		}
	}
	/* else {
		if !block.IsAirBlock(b3) {
			return false
		}
	}*/

	if _, ok := b.Model().(model.Empty); ok {
		if _, ok := b2.Model().(model.Empty); ok {
			return true
		}
	}
	return false
}

func (f Finder[Node]) Neighbours(no Node) (nodes []Node) {
	n := cube.Pos(no)

	n.Neighbours(func(neighbour cube.Pos) {
		if f.AllowStanding(neighbour) {
			nodes = append(nodes, Node(neighbour))
		}
	}, f.w.Range())
	return
}

//func (c *Client) FindPath(pos cube.Pos) astar.Path[cube.Pos] {
//	f := Finder[cube.Pos]{c.World(), c, true}
//
//	//path := astar.FindPath[cube.Pos](f, c., pos, maths.DistanceTo, maths.DistanceTo)
//	return path
//}
