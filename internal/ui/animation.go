package ui

import (
	"image/color"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/harmonica"
)

type AnimState struct {
	active      bool
	spring      harmonica.Spring
	offsetX     float64
	velocityX   float64
	targetX     float64
	flashColor  color.Color
	flashFrames int
	done        bool
}

func newAnimState() AnimState {
	return AnimState{
		spring: harmonica.NewSpring(harmonica.FPS(60), 6.0, 0.8),
	}
}

func (a *AnimState) start(targetX float64, c color.Color) {
	a.active = true
	a.done = false
	a.offsetX = 0
	a.velocityX = 0
	a.targetX = targetX
	a.flashColor = c
	a.flashFrames = 12
}

func (a *AnimState) tick() {
	if !a.active {
		return
	}
	a.offsetX, a.velocityX = a.spring.Update(a.offsetX, a.velocityX, a.targetX)
	if a.flashFrames > 0 {
		a.flashFrames--
	}
	if math.Abs(a.offsetX-a.targetX) < 1.0 && math.Abs(a.velocityX) < 0.5 {
		a.done = true
		a.active = false
	}
}

func animTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}
