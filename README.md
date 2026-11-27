# webvtt

Standard-library-only WebVTT parsing for Go. Cue payload markup is parsed into
nodes so `voice` and `lang` annotations become structured metadata instead of
visible subtitle text.

```go
doc, err := webvtt.ParseString(source)
if err != nil {
    log.Fatal(err)
}
for _, cue := range doc.Cues {
    fmt.Println(cue.Text("en-US"))
}
```

## Behaviour

- `Cue.Payload` keeps the original cue payload. Container and unknown nodes
  also retain their exact `Raw` source.
- `<v>` and `<lang>` annotations populate `Node.Voice` and `Node.Language`.
  Inherited values are applied while rendering.
- Language selection is case-insensitive and uses BCP 47 prefix fallback, so
  `en-US` can fall back to `en`. Untagged text is always retained. If no tagged
  text matches, all text is returned and `RenderResult.Fallback` is true.
- Unknown tags are preserved by default, including their annotations. Set
  `RenderOptions.OmitUnknown` to remove them. Timestamp tags are removed unless
  `KeepTimestamps` is set.
- `MergeOverlapping` only merges cues whose complete speaker sets are equal.
  Voiced and untagged cues, or cues for different voices, remain separate.

`ParseCuePayload`/`ParseCueMarkup` can be used without parsing a whole
document. `Render` returns text plus fallback and voice metadata.
