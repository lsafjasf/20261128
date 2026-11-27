# 20261128

Repository initialized for 20261128.

## stereo-subtitle

Stereoscopic subtitle parallax toolkit (Go):

- `stereo/projection.go` — eye-pair projection onto the zero-parallax screen
  plane; screen parallax (px) from depth, and the inverse.
- `stereo/disparity.go` — safe parallax budget; classifies each cue as normal
  stereo (`ok`) vs eye-strain risk (`strain-risk`). Uncrossed parallax is
  capped at the interocular-equivalent (divergence limit).
- `stereo/pair.go` — left/right subtitle pair. `MoveLeft`/`MoveRight` shift
  one eye and synchronously update the other (depth preserved);
  `FixDisparity` clamps unsafe disparity by correcting both eyes
  symmetrically around the pair center.
- `stereo/synth.go` — Go-synthesized, deterministic test cues mixing safe
  and unsafe disparities.

```sh
go test ./...
go run ./cmd/stereocheck -n 20
```
