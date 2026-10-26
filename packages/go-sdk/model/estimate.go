package model

import "math"

const asciiBytesPerEstimatedToken = 4

func EstimateTextTokens(text string) int {
	ascii := 0
	tokens := 0
	for _, value := range text {
		if value <= 0x7f {
			ascii++
		} else {
			tokens++
		}
	}
	asciiTokens := ascii / asciiBytesPerEstimatedToken
	if ascii%asciiBytesPerEstimatedToken != 0 {
		asciiTokens++
	}
	if asciiTokens > math.MaxInt-tokens {
		return math.MaxInt
	}
	return tokens + asciiTokens
}
