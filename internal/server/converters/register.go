// Package converters provides API format converters
package converters

func init() {
	// Register passthrough converter (no conversion)
	RegisterConverter(APIFormatPassthrough, APIFormatPassthrough, NewPassthroughConverter())

	// Register OpenAI to Anthropic converter (used when provider uses Anthropic API but client sends OpenAI format)
	RegisterConverter(APIFormatOpenAI, APIFormatAnthropic, NewOpenAIToAnthropicConverter())

	// Register Anthropic to OpenAI converter (used when provider uses OpenAI API but client sends Anthropic format)
	RegisterConverter(APIFormatAnthropic, APIFormatOpenAI, NewAnthropicToOpenAIConverter())
}
