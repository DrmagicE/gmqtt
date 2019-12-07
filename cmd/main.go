package main

import (
	"fmt"
	"math"
)

func escapeGhosts(ghosts [][]int, target []int) bool {
	min := math.Abs(float64(target[0])) + math.Abs(float64(target[1]))
	for _, v := range ghosts {
		tmp := math.Abs(float64(v[0]-target[0])) + math.Abs(float64(v[1]-target[1]))
		if tmp <= min {
			return false
		}
	}
	return true
}

func main() {

	fmt.Println(escapeGhosts([][]int{
		[]int{1, 0}, {0, 3},
	}, []int{0, 1}))
}
