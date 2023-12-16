package bot

import (
	"fmt"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/model"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/fzipp/astar"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"math"
	"slices"
	"time"
)

type Finder[Node cube.Pos] struct {
	w           *World
	c           *Client
	allowFlight bool
}

var blockList = []string{"minecraft:air", "minecraft:grass", "minecraft:seagrass", "minecraft:tallgrass"}

func WalkableBlock(b world.Block) bool {
	bb, _ := b.EncodeBlock()
	if slices.Contains(blockList, bb) {
		return true
	}
	return false
}

func (f Finder[_]) AllowStanding(n cube.Pos) bool {
	b := f.w.Block(n)
	b2 := f.w.Block(n.Side(cube.FaceUp))
	b3 := f.w.Block(n.Side(cube.FaceDown))

	if !f.allowFlight {
		if _, ok := b3.Model().(model.Empty); ok {
			return false
		}
	}

	if _, ok := b.Model().(model.Empty); ok {
		if _, ok := b2.Model().(model.Empty); ok {
			return true
		}
	}

	if WalkableBlock(b) && WalkableBlock(b2) {
		return true
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

func BlockPosFromVec3(position mgl32.Vec3) cube.Pos {
	position = vec3Floor(vec3Floor(position).Add(mgl32.Vec3{0.5, 0, 0.5}))

	pos := cube.Pos{int(position[0]), int(position[1]), int(position[2])}
	return pos
}

func (c *Client) FindPath(pos cube.Pos) astar.Path[cube.Pos] {
	f := Finder[cube.Pos]{c.World(), c, true}

	if !f.AllowStanding(pos) {
		return nil
	}
	position := c.Self.Position
	position = vec3Floor(vec3Floor(position).Add(mgl32.Vec3{0.5, 0, 0.5}))
	path := astar.FindPath[cube.Pos](f, cube.Pos{int(position[0]), int(position[1]), int(position[2])}, pos, DistanceTo, DistanceTo)
	return path
}

func DistanceTo(v cube.Pos, vec3d cube.Pos) float64 {
	xDiff, yDiff, zDiff := v.X()-vec3d.X(), v.Y()-vec3d.Y(), v.Z()-vec3d.Z()
	return math.Sqrt(float64(xDiff*xDiff + yDiff*yDiff + zDiff*zDiff))
}
func DistanceToVec3(v mgl32.Vec3, vec3d mgl32.Vec3) float64 {
	xDiff, yDiff, zDiff := v.X()-vec3d.X(), v.Y()-vec3d.Y(), v.Z()-vec3d.Z()
	return math.Sqrt(float64(xDiff*xDiff + yDiff*yDiff + zDiff*zDiff))
}

func (c *Client) FlyTo(position mgl32.Vec3) {
	defer c.flyLock.Unlock()
	c.flyLock.Lock()
	c.internalFlyTo(position)
}
func (c *Client) WalkTo(position mgl32.Vec3) {
	pos := cube.Pos{int(position[0]), int(position[1]), int(position[2])}
	paths := c.FindPath(pos)
	for _, path := range paths {
		c.SendCustomPosition(mgl32.Vec3{float32(path[0]) + 0.5, float32(path[1]) + 1.62, float32(path[2]) + 0.5})
		fmt.Println(path)
		time.Sleep(25 * time.Millisecond)
	}
	c.Self.Position = position
}
func FromBlockPos(v mgl32.Vec3) mgl32.Vec3 {
	newX := math.Floor(float64(v.X()))
	newY := math.Floor(float64(v.Y()))
	newZ := math.Floor(float64(v.Z()))
	if v.X() != 0 {
		//newX = newX + 0.5
		if v.X() > 0 {
			newX = newX + 0.5
		} else {
			newX = newX - 0.5
		}
	}
	//newY = newY
	if v.Z() != 0 {
		//newZ = newZ + 0.5
		if v.Z() > 0 {
			newZ = newZ + 0.5
		} else {
			newZ = newZ - 0.5
		}
	}

	return mgl32.Vec3{float32(newX), float32(newY), float32(newZ)}
}

var eyeY = mgl32.Vec3{0, 1.62, 0}

func (c *Client) SendCurrentPosition() {
	c.Conn.WritePacket(&packet.MovePlayer{
		Position:        c.Self.Position,
		Pitch:           c.Self.Pitch,
		Yaw:             c.Self.Yaw,
		HeadYaw:         c.Self.HeadYaw,
		EntityRuntimeID: c.Self.EntityRuntimeID,
		OnGround:        c.Self.OnGround,
	})
}

func (c *Client) SendCustomPosition(position mgl32.Vec3) {
	c.Conn.WritePacket(&packet.MovePlayer{
		Position:        position,
		Pitch:           c.Self.Pitch,
		Yaw:             c.Self.Yaw,
		HeadYaw:         c.Self.HeadYaw,
		EntityRuntimeID: c.Self.EntityRuntimeID,
		OnGround:        c.Self.OnGround,
	})
}

func (c *Client) internalFlyTo(position mgl32.Vec3) {
	nowPos := c.Self.Position
	position = position.Add(eyeY)
	vector := position.Sub(nowPos)
	magnitude := vector.Len()
	for magnitude > 1 {
		mV := vector.Mul(1 / magnitude)
		mV = mV.Mul(1)
		nowPos = nowPos.Add(mV)

		time.Sleep(5 * time.Millisecond)
		//c.Self.Position.SendCustomPosition(c, nowPos)
		c.SendCurrentPosition()
		c.Self.Position = nowPos
		//c.Player.SetPosition(c.Player.Position.X, c.Player.Position.Y, c.Player.Position.Z)
		vector = position.Sub(nowPos)
		magnitude = vector.Len()
	}
	time.Sleep(5 * time.Millisecond)
	c.SendCurrentPosition()
	c.Self.Position = position
	//c.Player.SetPosition(position.X, position.Y, position.Z)
	//c.Player.SendCustomPosition(c, position)
}
func vec3Floor(v mgl32.Vec3) mgl32.Vec3 {
	vec3 := vec32To64(v)
	return vec64To32(mgl64.Vec3{math.Floor(vec3[0]), math.Floor(vec3[1]), math.Floor(vec3[2])})
}
