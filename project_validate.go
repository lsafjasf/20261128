package marker

func (p *Project) validateClip(clip Clip) []Diagnostic {
	return validateClipAgainst(clip, p.media, p.sequences)
}

func (p *Project) validateMarker(candidate Marker, allowMissingOwner bool) []Diagnostic {
	return validateMarkerAgainst(candidate, p.media, p.sequences, p.clips)
}
