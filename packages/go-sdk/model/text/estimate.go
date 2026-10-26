package text

const ASCIIBytesPerEstimatedToken = 4

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
	asciiTokens := ascii / ASCIIBytesPerEstimatedToken
	if ascii%ASCIIBytesPerEstimatedToken != 0 {
		asciiTokens++
	}
	return SaturatedAdd(tokens, asciiTokens)
}
