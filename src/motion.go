package main

import (
	"crypto/rand"
	"math"
)

const (
	// Minecraft coordinate limits for simulation
	fieldSize = 2000 // 2000x2000 field
	minY      = 85   // Ground level
	maxY      = 110  // Hills
)

// MotionGenerator handles realistic player movement simulation
type MotionGenerator struct {
	X, Y, Z float64
	Angle   float64
	Speed   float64
}

func NewMotionGenerator() *MotionGenerator {
	// Start with random position within the field
	return &MotionGenerator{
		X:     float64(getSecureRandomInt(fieldSize)),
		Y:     95.0, // Average ground level
		Z:     float64(getSecureRandomInt(fieldSize)),
		Angle: getRandomFloat() * 2 * math.Pi,
		Speed: 2.0 + getRandomFloat()*3.0,
	}
}

// Update calculates the next position based on random walk logic
func (m *MotionGenerator) Update() {
	// Slightly change direction (random walk)
	angleChange := (getRandomFloat() - 0.5) * 0.3 // Small turns
	m.Angle += angleChange

	// Sometimes change direction more dramatically
	if getRandomFloat() < 0.05 {
		m.Angle += (getRandomFloat() - 0.5) * math.Pi
	}

	// Occasionally change speed (running/walking)
	if getRandomFloat() < 0.1 {
		m.Speed = 2.0 + getRandomFloat()*3.0
	}

	// Move in the current direction
	m.X += math.Cos(m.Angle) * m.Speed
	m.Z += math.Sin(m.Angle) * m.Speed

	// Bounce off boundaries
	if m.X < 0 {
		m.X = 0
		m.Angle = math.Pi - m.Angle
	} else if m.X > float64(fieldSize) {
		m.X = float64(fieldSize)
		m.Angle = math.Pi - m.Angle
	}

	if m.Z < 0 {
		m.Z = 0
		m.Angle = -m.Angle
	} else if m.Z > float64(fieldSize) {
		m.Z = float64(fieldSize)
		m.Angle = -m.Angle
	}

	// Generate terrain height (gentle hills)
	terrainHeight := generateTerrainHeight(m.X, m.Z)

	// Smoothly adjust Y to terrain
	m.Y += (terrainHeight - m.Y) * 0.2

	// Keep Y in bounds
	if m.Y < float64(minY) {
		m.Y = float64(minY)
	} else if m.Y > float64(maxY) {
		m.Y = float64(maxY)
	}
}

func generateTerrainHeight(x, z float64) float64 {
	// Simple Perlin-like noise using sine waves
	scale := 100.0
	height := float64(minY) + float64(maxY-minY)/2.0

	// Multiple frequency waves for varied terrain
	height += math.Sin(x/scale)*5.0 + math.Cos(z/scale)*5.0
	height += math.Sin(x/(scale*2))*3.0 + math.Cos(z/(scale*2))*3.0
	height += math.Sin((x+z)/(scale*0.5)) * 2.0

	return height
}

// Returns a secure random float between 0.0 and 1.0
func getRandomFloat() float64 {
	b := make([]byte, 2)
	rand.Read(b)
	// Convert 16 bits to float 0..1
	val := uint16(b[0])<<8 | uint16(b[1])
	return float64(val) / 65535.0
}
