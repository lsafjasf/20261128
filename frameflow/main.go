package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Println(`用法:
  frameflow simulate -o logs.jsonl [-seed 1]   生成四机位仿真日志
  frameflow analyze logs.jsonl                 合并时间线并定位首个偏离子系统`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "simulate":
		out, seed := "logs.jsonl", int64(1)
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "-o":
				i++
				out = os.Args[i]
			case "-seed":
				i++
				fmt.Sscanf(os.Args[i], "%d", &seed)
			}
		}
		err = runSim(out, seed)
	case "analyze":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		err = analyze(os.Args[2])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
