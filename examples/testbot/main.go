package main

import "os"

func main() {
	mode := "random"
	if len(os.Args) > 1 {
		mode = os.Args[1]
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	switch mode {
	case "random":
		runRandom()
	case "complex":
		runComplex()
	default:
		os.Stderr.WriteString("unknown mode: " + mode + "\n")
		os.Exit(1)
	}
}
