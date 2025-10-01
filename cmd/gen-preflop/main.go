package main

import (
	"flag"
	"os"

	"github.com/lox/pokerforbots/v2/sdk/analysis"
)

func main() {
	simulations := flag.Int("simulations", 10000, "Number of simulations per hand")
	output := flag.String("output", "preflop_gen.go", "Output file for generated Go code")
	flag.Parse()

	// Generate the preflop table
	table := analysis.GeneratePreflopTable(*simulations)

	// Generate Go code
	code := table.GenerateGoCode()
	err := os.WriteFile(*output, []byte(code), 0644)
	if err != nil {
		os.Exit(1)
	}
}
