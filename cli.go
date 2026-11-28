package wavinfo

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CLIError carries an exit code for command-line failures.
type CLIError struct {
	Code int
	Msg  string
}

func (e *CLIError) Error() string { return e.Msg }

// CLIOptions controls a wavinfo command invocation.
type CLIOptions struct {
	Args       []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Preference Preference
}

// RunCLI executes a wavinfo subcommand and returns an error carrying the
// intended process exit code on failure.
func RunCLI(opts CLIOptions) error {
	if len(opts.Args) == 0 {
		return &CLIError{Code: 2, Msg: "wavinfo: missing subcommand (inspect|get|set|delete|apply|fields)"}
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	cmd, args := opts.Args[0], opts.Args[1:]
	switch cmd {
	case "fields":
		return cmdFields(stdout)
	case "inspect":
		path, jsonOut, err := onePath(args, "-json")
		if err != nil {
			return err
		}
		return cmdInspect(path, jsonOut, stdout)
	case "get":
		if len(args) != 2 {
			return usageErr("usage: wavinfo get FILE FIELD")
		}
		return cmdGet(args[0], args[1], stdout)
	case "set":
		if len(args) != 3 {
			return usageErr("usage: wavinfo set FILE FIELD VALUE")
		}
		return cmdSet(args[0], args[1], args[2])
	case "delete":
		if len(args) != 2 {
			return usageErr("usage: wavinfo delete FILE FIELD")
		}
		return cmdDelete(args[0], args[1])
	case "apply":
		path, _, err := onePath(args, "-json")
		if err != nil {
			return err
		}
		return cmdApply(path, opts.Preference, opts.Stdin, stdout)
	default:
		return usageErr(fmt.Sprintf("wavinfo: unknown subcommand %q", cmd))
	}
}

func usageErr(msg string) error {
	return &CLIError{Code: 2, Msg: msg}
}

func onePath(args []string, flag string) (path string, flagOn bool, err error) {
	for _, a := range args {
		if a == flag {
			flagOn = true
			continue
		}
		if path != "" {
			return "", false, usageErr("wavinfo: multiple file paths given")
		}
		path = a
	}
	if path == "" {
		return "", false, usageErr("wavinfo: missing FILE argument")
	}
	return path, flagOn, nil
}

func cmdFields(out io.Writer) error {
	for _, id := range canonicalOrder {
		name, _ := FieldName(id)
		fmt.Fprintf(out, "%s\t%s\n", id, name)
	}
	return nil
}

type inspectEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Known bool   `json:"known"`
	Text  string `json:"text"`
}

func cmdInspect(path string, asJSON bool, out io.Writer) error {
	file, err := openFile(path)
	if err != nil {
		return err
	}
	defer file.Close()

	wav, err := Parse(file)
	if err != nil {
		return &CLIError{Code: 1, Msg: err.Error()}
	}
	entries := wav.Info().Entries()
	if !asJSON {
		for _, e := range entries {
			marker := " "
			if !e.Known() {
				marker = "~"
			}
			fmt.Fprintf(out, "%s %s\t%s\n", marker, e.ID, e.Text)
		}
		return nil
	}

	rows := make([]inspectEntry, 0, len(entries))
	for _, e := range entries {
		name, _ := FieldName(e.ID)
		rows = append(rows, inspectEntry{ID: e.ID, Name: name, Known: e.Known(), Text: e.Text})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func cmdGet(path, key string, out io.Writer) error {
	id, err := resolveCLIID(key)
	if err != nil {
		return err
	}
	file, err := openFile(path)
	if err != nil {
		return err
	}
	defer file.Close()

	wav, err := Parse(file)
	if err != nil {
		return &CLIError{Code: 1, Msg: err.Error()}
	}
	text, ok := wav.Info().Get(id)
	if !ok {
		return &CLIError{Code: 1, Msg: fmt.Sprintf("wavinfo: field %s not found", id)}
	}
	_, err = fmt.Fprintln(out, text)
	return err
}

func cmdSet(path, key, value string) error {
	id, err := resolveCLIID(key)
	if err != nil {
		return err
	}
	return rewrite(path, func(wav *File) error {
		if err := wav.Info().Set(id, value); err != nil {
			return &CLIError{Code: 2, Msg: err.Error()}
		}
		return nil
	})
}

func cmdDelete(path, key string) error {
	id, err := resolveCLIID(key)
	if err != nil {
		return err
	}
	return rewrite(path, func(wav *File) error {
		if !wav.Info().Delete(id) {
			return &CLIError{Code: 1, Msg: fmt.Sprintf("wavinfo: field %s not found", id)}
		}
		return nil
	})
}

// resolveCLIID accepts any project key but, for direct set/get/delete use,
// requires a resolvable or four-byte INFO identifier.
func resolveCLIID(key string) (string, error) {
	id, ok := ResolveField(key)
	if ok {
		return id, nil
	}
	if len(id) == 4 && isPrintableASCII(id) {
		return toASCIIUpper(id), nil
	}
	return "", usageErr(fmt.Sprintf("wavinfo: cannot resolve field %q", key))
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func cmdApply(path string, pref Preference, in io.Reader, out io.Writer) error {
	var reader io.Reader
	if in == nil {
		in = os.Stdin
	}
	reader = in

	var fields map[string]string
	if err := json.NewDecoder(reader).Decode(&fields); err != nil {
		return &CLIError{Code: 2, Msg: "wavinfo: invalid JSON input: " + err.Error()}
	}

	return rewriteWithReport(path, func(wav *File) (*ApplyResult, error) {
		fileNameValues := ParseFileName(filepath.Base(path))
		return wav.ApplyProjectFields(fields, fileNameValues, pref)
	}, out)
}

func rewrite(path string, mutate func(*File) error) error {
	err := rewriteWithReport(path, func(wav *File) (*ApplyResult, error) {
		return nil, mutate(wav)
	}, io.Discard)
	return err
}

func rewriteWithReport(path string, mutate func(*File) (*ApplyResult, error), out io.Writer) error {
	file, err := openFile(path)
	if err != nil {
		return err
	}
	wav, perr := Parse(file)
	file.Close()
	if perr != nil {
		return &CLIError{Code: 1, Msg: perr.Error()}
	}

	res, merr := mutate(wav)
	if merr != nil {
		return merr
	}
	if err := atomicWrite(path, wav); err != nil {
		return err
	}
	if res != nil {
		printApplyReport(res, out)
	}
	return nil
}

func printApplyReport(res *ApplyResult, out io.Writer) {
	if len(res.Applied) > 0 {
		fmt.Fprintf(out, "applied: %s\n", strings.Join(res.Applied, ", "))
	}
	for _, c := range res.Conflicted {
		fmt.Fprintf(out, "conflict: %s embedded=%q external=%q winner=%s\n",
			c.ID, c.Embedded, c.External, c.Winner)
	}
	if len(res.Skipped) > 0 {
		sort.Strings(res.Skipped)
		fmt.Fprintf(out, "skipped (unresolved): %s\n", strings.Join(res.Skipped, ", "))
	}
}

func atomicWrite(path string, wav *File) error {
	dir, name := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, ".wavinfo-"+name+"-*")
	if err != nil {
		return &CLIError{Code: 1, Msg: err.Error()}
	}
	tmpName := tmp.Name()
	if _, err := wav.WriteTo(tmp); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return &CLIError{Code: 1, Msg: err.Error()}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return &CLIError{Code: 1, Msg: err.Error()}
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return &CLIError{Code: 1, Msg: err.Error()}
	}
	return nil
}

func openFile(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, &CLIError{Code: 1, Msg: err.Error()}
	}
	return f, nil
}
