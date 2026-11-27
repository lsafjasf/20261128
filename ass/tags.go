package ass

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type prop uint8

const (
	propScaleX prop = iota
	propScaleY
	propRotateX
	propRotateY
	propRotateZ
	propPosX
	propPosY
	propOriginX
	propOriginY
	propAlignment
)

type animation struct {
	start   time.Duration
	end     time.Duration
	accel   float64
	order   int
	changes map[prop]float64
	initial map[prop]float64
}

type timeline struct {
	base       Transform
	animations []*animation
	nextOrder  int
}

type parsedLine struct {
	plain    string
	timeline timeline
}

func parseLine(text string) (parsedLine, error) {
	line := parsedLine{timeline: timeline{base: DefaultTransform()}}
	var plain strings.Builder
	for index := 0; index < len(text); {
		if text[index] != '{' {
			next := strings.IndexByte(text[index:], '{')
			if next < 0 {
				plain.WriteString(text[index:])
				break
			}
			plain.WriteString(text[index : index+next])
			index += next
			continue
		}
		end := strings.IndexByte(text[index+1:], '}')
		if end < 0 {
			plain.WriteString(text[index:])
			break
		}
		end += index + 1
		if err := parseOverrideBlock(text[index+1:end], &line.timeline); err != nil {
			return parsedLine{}, err
		}
		index = end + 1
	}
	line.plain = cleanPlainText(plain.String())
	return line, nil
}

func cleanPlainText(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 >= len(text) {
			out.WriteByte(text[i])
			continue
		}
		switch text[i+1] {
		case 'N', 'n':
			out.WriteByte('\n')
			i++
		case 'h':
			out.WriteByte(' ')
			i++
		default:
			out.WriteByte(text[i])
		}
	}
	return out.String()
}

func parseOverrideBlock(block string, line *timeline) error {
	_, err := parseTagSequence(block, line, true)
	return err
}

// parseTagSequence walks one override block or one \t target sequence. Nested
// parentheses are consumed by readTag, so nested \t tags are visited recursively.
func parseTagSequence(sequence string, line *timeline, applyBase bool) (map[prop]float64, error) {
	changes := make(map[prop]float64)
	for i := 0; i < len(sequence); {
		if sequence[i] != '\\' {
			i++
			continue
		}
		name, args, next := readTag(sequence, i)
		i = next
		if name == "" {
			continue
		}
		tagName := strings.ToLower(name)
		if tagName == "t" {
			if err := parseTransition(args, line); err != nil {
				return nil, err
			}
			continue
		}
		if tagName == "move" {
			anim, err := parseMove(args, line)
			if err != nil {
				return nil, err
			}
			if anim != nil {
				line.animations = append(line.animations, anim)
			}
			continue
		}
		values, err := parseStaticTag(tagName, args)
		if err != nil {
			return nil, err
		}
		for key, value := range values {
			changes[key] = value
			if applyBase {
				applyProperty(&line.base, key, value)
			}
		}
	}
	return changes, nil
}

func readTag(s string, start int) (name, args string, next int) {
	i := start + 1
	nameStart := i
	if i < len(s) && s[i] >= '0' && s[i] <= '9' {
		// Colours use names such as \1c. Ordinary tags stop before their value.
		for i < len(s) && (isLetterByte(s[i]) || s[i] >= '0' && s[i] <= '9') {
			i++
		}
	} else {
		for i < len(s) && isLetterByte(s[i]) {
			i++
		}
	}
	name = s[nameStart:i]
	if i < len(s) && s[i] == '(' {
		end := matchingParen(s, i)
		if end < 0 {
			return name, strings.TrimSpace(s[i+1:]), len(s)
		}
		return name, strings.TrimSpace(s[i+1 : end]), end + 1
	}
	valueStart := i
	for i < len(s) && s[i] != '\\' {
		i++
	}
	return name, strings.TrimSpace(s[valueStart:i]), i
}

func isLetterByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func matchingParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseTransition(args string, line *timeline) error {
	parts := splitTopLevel(args, ',')
	if len(parts) == 0 {
		return nil
	}
	var start, end time.Duration
	accel := 1.0
	target := ""
	if strings.HasPrefix(strings.TrimSpace(parts[0]), "\\") {
		start = 0
		end = -1
		target = args
	} else {
		if len(parts) < 3 {
			return fmt.Errorf("ass: malformed \\t(%s)", args)
		}
		var err error
		start, err = parseTagTime(parts[0])
		if err != nil {
			return err
		}
		end, err = parseTagTime(parts[1])
		if err != nil {
			return err
		}
		third := strings.TrimSpace(parts[2])
		if len(parts) >= 4 && !strings.HasPrefix(third, "\\") {
			accel, err = strconv.ParseFloat(third, 64)
			if err != nil {
				return fmt.Errorf("ass: invalid \\t acceleration %q", third)
			}
			target = strings.Join(parts[3:], ",")
		} else {
			target = strings.Join(parts[2:], ",")
		}
	}
	changes, err := parseTagSequence(target, line, false)
	if err != nil {
		return err
	}
	if len(changes) == 0 || end == start {
		return nil
	}
	line.animations = append(line.animations, &animation{
		start:   start,
		end:     end,
		accel:   accel,
		order:   line.nextOrder,
		changes: changes,
	})
	line.nextOrder++
	return nil
}

func parseMove(args string, line *timeline) (*animation, error) {
	parts := splitTopLevel(args, ',')
	if len(parts) < 4 {
		return nil, fmt.Errorf("ass: malformed \\move(%s)", args)
	}
	values := make([]float64, 4)
	for i := 0; i < 4; i++ {
		value, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err != nil {
			return nil, fmt.Errorf("ass: invalid \\move value %q", parts[i])
		}
		values[i] = value
	}
	start, end := time.Duration(0), time.Duration(-1)
	if len(parts) >= 6 {
		var err error
		start, err = parseTagTime(parts[4])
		if err != nil {
			return nil, err
		}
		end, err = parseTagTime(parts[5])
		if err != nil {
			return nil, err
		}
	}
	changes := map[prop]float64{propPosX: values[2], propPosY: values[3]}
	return &animation{
		start:   start,
		end:     end,
		accel:   1,
		order:   line.nextOrder,
		changes: changes,
		initial: map[prop]float64{
			propPosX: values[0],
			propPosY: values[1],
		},
	}, nil
}

func parseStaticTag(name, args string) (map[prop]float64, error) {
	value := strings.TrimSpace(args)
	parseNumber := func() (float64, error) {
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("ass: invalid \\%s value %q", name, args)
		}
		return number, nil
	}
	switch strings.ToLower(name) {
	case "fscx":
		number, err := parseNumber()
		return map[prop]float64{propScaleX: number}, err
	case "fscy":
		number, err := parseNumber()
		return map[prop]float64{propScaleY: number}, err
	case "frz", "fr":
		number, err := parseNumber()
		return map[prop]float64{propRotateZ: number}, err
	case "frx":
		number, err := parseNumber()
		return map[prop]float64{propRotateX: number}, err
	case "fry":
		number, err := parseNumber()
		return map[prop]float64{propRotateY: number}, err
	case "an":
		number, err := parseNumber()
		return map[prop]float64{propAlignment: number}, err
	case "pos":
		parts := splitTopLevel(args, ',')
		if len(parts) != 2 {
			return nil, fmt.Errorf("ass: malformed \\pos(%s)", args)
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("ass: invalid \\pos value %q", parts[0])
		}
		y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("ass: invalid \\pos value %q", parts[1])
		}
		return map[prop]float64{propPosX: x, propPosY: y}, nil
	case "org":
		parts := splitTopLevel(args, ',')
		if len(parts) != 2 {
			return nil, fmt.Errorf("ass: malformed \\org(%s)", args)
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("ass: invalid \\org value %q", parts[0])
		}
		y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("ass: invalid \\org value %q", parts[1])
		}
		return map[prop]float64{propOriginX: x, propOriginY: y}, nil
	}
	return nil, nil
}

func applyProperty(state *Transform, key prop, value float64) {
	switch key {
	case propScaleX:
		state.ScaleX = value
	case propScaleY:
		state.ScaleY = value
	case propRotateX:
		state.RotateX = value
	case propRotateY:
		state.RotateY = value
	case propRotateZ:
		state.RotateZ = value
	case propPosX, propPosY:
		if state.Position == nil {
			state.Position = &Point{}
		}
		if key == propPosX {
			state.Position.X = value
		} else {
			state.Position.Y = value
		}
	case propOriginX, propOriginY:
		if state.Origin == nil {
			state.Origin = &Point{}
		}
		if key == propOriginX {
			state.Origin.X = value
		} else {
			state.Origin.Y = value
		}
	case propAlignment:
		state.Alignment = int(math.Round(value))
	}
}

func parseTagTime(value string) (time.Duration, error) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("ass: invalid tag time %q", value)
	}
	if number < 0 {
		number = 0
	}
	return time.Duration(math.Round(number)) * time.Millisecond, nil
}

func splitTopLevel(s string, separator byte) []string {
	var parts []string
	start, depth := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if s[i] == separator && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}
