package tunnel

import (
	"fmt"
	"math/rand"
)

var colors = []string{
	"red", "blue", "green", "yellow", "purple", "orange",
	"pink", "brown", "black", "white", "gray", "cyan",
}

var animals = []string{
	"cat", "dog", "bird", "fish", "lion", "tiger",
	"bear", "wolf", "fox", "deer", "rabbit", "mouse",
}

func generateTunnelName() string {
	color := colors[rand.Intn(len(colors))]
	animal := animals[rand.Intn(len(animals))]
	number := rand.Intn(900) + 100
	return fmt.Sprintf("%s-%s-%d", color, animal, number)
}
