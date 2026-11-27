# 20261128

Repository initialized for 20261128.

## Caption Session

Go implementation of a versioned live-caption session with delay
compensation, scoped local replacement, freeze-based no-backfill semantics
and disconnect/reconnect support. See `DESIGN.md` for the full design and
`cmd/captiondemo` for a runnable walkthrough.

```
go test -race ./...
go run ./cmd/captiondemo
```
