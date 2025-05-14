package main

import (
	"context"
	"encoding/json"
	"fmt"

	ollama "github.com/ollama/ollama/api"
)

func main() {
	ctx := context.Background()

	c, err := ollama.ClientFromEnvironment()
	if err != nil {
		panic(err)
	}

	// fmt.Println("(Regular chat, no tools / formatting...)")
	// if err := c.Chat(ctx,
	// 	&ollama.ChatRequest{
	// 		Model: "qwen3",
	// 		Messages: []ollama.Message{
	// 			{Role: "system", Content: "You're a helpful poetry-writing assistant. Think through your tasks out loud. Don't think too hard, though."},
	// 			{Role: "user", Content: "Write a haiku about dogs"},
	// 		},
	// 	},
	// 	func(res ollama.ChatResponse) error {
	// 		// fmt.Print(".")
	// 		if res.Message.Content != "" {
	// 			fmt.Printf("%s", res.Message.Content)
	// 		}
	// 		if len(res.Message.ToolCalls) > 0 {
	// 			fmt.Println()
	// 			for _, call := range res.Message.ToolCalls {
	// 				fmt.Printf("[%d] %s(%#v)\n",
	// 					call.Function.Index,
	// 					call.Function.Name,
	// 					call.Function.Arguments,
	// 				)
	// 			}
	// 		}
	// 		if res.Done {
	// 			fmt.Println("\n[DONE]")
	// 		}
	// 		return nil
	// 	},
	// ); err != nil {
	// 	panic(err)
	// }

	fmt.Println("(Try using tool calls...)")
	if err := c.Chat(ctx,
		&ollama.ChatRequest{
			Model: "qwen3",
			Messages: []ollama.Message{
				{Role: "system", Content: "You're a helpful poetry-writing assistant. Think through your tasks out loud. Call functions freely – they're helpful and reliable. Before calling a function, explain your thinking. After calling the function, explain how it affects your thinking."},
				{Role: "user", Content: "Write a haiku about dogs"},
			},
			Tools: []ollama.Tool{
				{
					Type: "function",
					Function: ollama.ToolFunction{
						Name:        "get_dog_fact",
						Description: "Get a random fact about dogs that may be relevant when writing a poem.",
					},
				},
			},
		},
		func(res ollama.ChatResponse) error {
			// fmt.Print(".")
			if res.Message.Content != "" {
				fmt.Printf("%s", res.Message.Content)
			}
			if len(res.Message.ToolCalls) > 0 {
				fmt.Println()
				for _, call := range res.Message.ToolCalls {
					fmt.Printf("[%d] %s(%#v)\n",
						call.Function.Index,
						call.Function.Name,
						call.Function.Arguments,
					)
				}
			}
			if res.Done {
				fmt.Println("\n[DONE]")
			}
			return nil
		},
	); err != nil {
		panic(err)
	}

	fmt.Println("(Try using formatting...)")
	if err := c.Chat(ctx,
		&ollama.ChatRequest{
			Model: "qwen3",
			Messages: []ollama.Message{
				{Role: "system", Content: "You're a helpful poetry-writing assistant. Think through your tasks out loud. Call functions freely – they're helpful and reliable. Before calling a function, explain your thinking. After calling the function, explain how it affects your thinking."},
				{Role: "user", Content: "Write a haiku about dogs"},
			},
			Format: json.RawMessage(`{
				"oneOf": [
					{
						"type": "object",
						"properties": {
							"message_type": {
								"const": "thought_process"
							},
							"thought_process": {
								"type": "string",
								"description": "The internal thought process of the assistant."
							}
						},
						"required": ["message_type", "thought_process"]
					},
					{
						"type": "object",
						"properties": {
							"message_type": {
								"const": "function_call"
							},
							"function_call": {
								"enum": [
									"get_random_number",
									"get_dog_fact",
									"get_cat_fact",
									"get_bird_fact"
								],
								"description": "The function call made by the assistant"
							}
						},
						"required": ["message_type", "function_call"]
					},
					{
						"type": "object",
						"properties": {
							"message_type": {
								"const": "assistant_message"
							},
							"assistant_message": {
								"type": "string",
								"description": "The assistant's message to be passed to the user."
							}
						},
						"required": ["message_type", "assistant_message"]
					}
			}`),
		},
		func(res ollama.ChatResponse) error {
			// fmt.Print(".")
			if res.Message.Content != "" {
				fmt.Printf("%s", res.Message.Content)
			}
			if len(res.Message.ToolCalls) > 0 {
				fmt.Println()
				for _, call := range res.Message.ToolCalls {
					fmt.Printf("[%d] %s(%#v)\n",
						call.Function.Index,
						call.Function.Name,
						call.Function.Arguments,
					)
				}
			}
			if res.Done {
				fmt.Println("\n[DONE]")
			}
			return nil
		},
	); err != nil {
		panic(err)
	}

	// // Make the CLI app
	// app := makeApp()

	// // Run it
	// if err := app.Run(ctx, os.Args); err != nil {
	// 	panic(err)
	// }

}
