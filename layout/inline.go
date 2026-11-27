package layout

import (
	"strings"
)

type inlineKind uint8

const (
	itemText inlineKind = iota
	itemRuby
	itemBreak
)

type inlineItem struct {
	kind   inlineKind
	text   string
	layers []string
}

func parseInline(node *xmlNode) []inlineItem {
	var items []inlineItem
	for i := 0; i < len(node.Parts); i++ {
		part := node.Parts[i]
		if part.Node == nil {
			items = append(items, splitTextRun(part.Text)...)
			continue
		}
		switch part.Node.Name.Local {
		case "br":
			items = append(items, inlineItem{kind: itemBreak})
		case "ruby":
			items = append(items, parseRuby(part.Node)...)
		case "rt", "rp":
			continue
		case "metadata", "set", "animate", "animateTransform", "animateMotion":
			continue
		default:
			switch rubyAttr(part.Node) {
			case "container":
				items = append(items, parseRubyContainer(part.Node)...)
				continue
			case "base", "basecontainer":
				base := parseInline(part.Node)
				last := i
				for j := i + 1; j < len(node.Parts); j++ {
					next := node.Parts[j]
					if next.Node == nil && strings.TrimSpace(next.Text) == "" {
						continue
					}
					if next.Node == nil || rubyAttr(next.Node) != "text" && rubyAttr(next.Node) != "textcontainer" {
						break
					}
					if annotation := annotationText(next.Node); annotation != "" {
						base = addRubyLayer(base, annotation)
					}
					last = j
				}
				items = append(items, base...)
				i = last
				continue
			case "text", "textcontainer":
				continue
			}
			items = append(items, parseInline(part.Node)...)
		}
	}
	return normalizeInline(items)
}

func rubyAttr(node *xmlNode) string {
	for _, attr := range node.Attrs {
		if attr.Name.Local == "ruby" {
			return strings.ToLower(strings.TrimSpace(attr.Value))
		}
	}
	return ""
}

func parseRubyContainer(node *xmlNode) []inlineItem {
	var base []inlineItem
	var annotations []string
	for _, part := range node.Parts {
		if part.Node == nil {
			if strings.TrimSpace(part.Text) != "" {
				base = append(base, splitTextRun(part.Text)...)
			}
			continue
		}
		switch rubyAttr(part.Node) {
		case "base", "basecontainer":
			base = append(base, parseInline(part.Node)...)
		case "text", "textcontainer":
			if annotation := annotationText(part.Node); annotation != "" {
				annotations = append(annotations, annotation)
			}
		case "container":
			base = append(base, parseRubyContainer(part.Node)...)
		default:
			base = append(base, parseInline(part.Node)...)
		}
	}
	base = normalizeInline(base)
	for _, annotation := range annotations {
		base = addRubyLayer(base, annotation)
	}
	return base
}

func parseRuby(node *xmlNode) []inlineItem {
	var base []inlineItem
	var annotations []string
	for _, part := range node.Parts {
		if part.Node != nil && part.Node.Name.Local == "rt" {
			if text := annotationText(part.Node); text != "" {
				annotations = append(annotations, text)
			}
			continue
		}
		if part.Node == nil {
			base = append(base, splitTextRun(part.Text)...)
			continue
		}
		if part.Node.Name.Local == "rp" {
			continue
		}
		base = append(base, parseInline(part.Node)...)
	}
	base = normalizeInline(base)
	for _, annotation := range annotations {
		base = addRubyLayer(base, annotation)
	}
	return base
}

func annotationText(node *xmlNode) string {
	var builder strings.Builder
	var visit func(*xmlNode)
	visit = func(current *xmlNode) {
		for _, part := range current.Parts {
			if part.Node == nil {
				builder.WriteString(part.Text)
				continue
			}
			if part.Node.Name.Local == "rt" || part.Node.Name.Local == "rp" {
				continue
			}
			visit(part.Node)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func addRubyLayer(base []inlineItem, annotation string) []inlineItem {
	if len(base) == 0 || annotation == "" {
		return base
	}
	if len(base) == 1 {
		if base[0].kind == itemText && strings.TrimSpace(base[0].text) == "" {
			return base
		}
		base[0].layers = append(base[0].layers, annotation)
		return base
	}

	var clusters []inlineItem
	for _, item := range base {
		clusters = append(clusters, explodeRubyItem(item)...)
	}
	if len(clusters) == 0 {
		return base
	}
	weights := make([]float64, len(clusters))
	for i := range weights {
		weights[i] = 1
	}
	pieces := distributeClusters(annotation, weights)
	for i := range clusters {
		if pieces[i] != "" {
			clusters[i].layers = append(clusters[i].layers, pieces[i])
		}
	}
	return clusters
}

func explodeRubyItem(item inlineItem) []inlineItem {
	if item.kind != itemRuby && item.kind != itemText {
		return []inlineItem{item}
	}
	clusters := graphemes(item.text)
	if len(clusters) <= 1 {
		return []inlineItem{item}
	}
	result := make([]inlineItem, len(clusters))
	weights := make([]float64, len(clusters))
	for i, cluster := range clusters {
		result[i] = inlineItem{kind: itemText, text: cluster}
		weights[i] = 1
	}
	for layerIndex, layer := range item.layers {
		pieces := distributeClusters(layer, weights)
		for i, piece := range pieces {
			if piece == "" {
				continue
			}
			if len(result[i].layers) <= layerIndex {
				result[i].layers = append(result[i].layers, make([]string, layerIndex-len(result[i].layers)+1)...)
			}
			result[i].layers[layerIndex] = piece
		}
	}
	for i := range result {
		if len(result[i].layers) > 0 {
			result[i].kind = itemRuby
		}
	}
	return result
}

func normalizeInline(items []inlineItem) []inlineItem {
	result := make([]inlineItem, 0, len(items))
	for _, item := range items {
		if item.kind == itemText && item.text == " " {
			if len(result) == 0 {
				continue
			}
			last := result[len(result)-1]
			if last.kind == itemBreak || (last.kind == itemText && last.text == " ") {
				continue
			}
		}
		if item.kind == itemBreak {
			for len(result) > 0 && result[len(result)-1].kind == itemText && result[len(result)-1].text == " " {
				result = result[:len(result)-1]
			}
			if len(result) > 0 && result[len(result)-1].kind == itemBreak {
				continue
			}
		}
		result = append(result, item)
	}
	for len(result) > 0 && result[len(result)-1].kind == itemText && result[len(result)-1].text == " " {
		result = result[:len(result)-1]
	}
	return result
}
