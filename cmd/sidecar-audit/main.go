// sidecar-audit 比较多份 checksum sidecar,建立文件身份与证据链。
//
// 用法:
//
//	sidecar-audit [flags] <sidecar...>
//
// 退出码: 0 = pass, 1 = 阻断(同名冲突/证据矛盾/strict 下的不可判定), 2 = 运行错误。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"sidecaraudit/internal/audit"
)

func main() {
	payload := flag.String("payload", "", "载荷目录,提供后对每条断言重算核验")
	asJSON := flag.Bool("json", false, "以 JSON 输出完整报告")
	strict := flag.Bool("strict", false, "不可判定项(未知算法等)也阻断发布")
	flag.Parse()

	files := flag.Args()
	if len(files) < 2 {
		fmt.Fprintln(os.Stderr, "用法: sidecar-audit [-payload DIR] [-json] [-strict] <sidecar...> (至少两份)")
		os.Exit(2)
	}

	rep, err := audit.Run(files, audit.Options{PayloadRoot: *payload, Strict: *strict})
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(2)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			os.Exit(2)
		}
	} else {
		audit.RenderText(os.Stdout, rep)
	}

	if rep.Verdict == "blocked" {
		os.Exit(1)
	}
}
