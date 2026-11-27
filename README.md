# 20261128

Repository initialized for 20261128.

## frameline

Rebuild the real timeline of an exported image sequence whose frame numbers
contain gaps. A player that sorts by file name plays the remaining frames
back-to-back and appears to speed up; this tool anchors sequence numbers to
reference timestamps instead, so gaps keep their true duration. Duration is
derived from sequence numbers, never from the number of exported files.

Standard library only. Usage:

```sh
# explicit frame rate + anchor timestamp (defaults to first exported frame)
go run . -dir ./frames -fps 24 -start 2026-11-28T10:00:00Z

# or derive the frame rate from two reference timestamps
go run . -dir ./frames -start 2026-11-28T10:00:00Z \
    -ref2 2026-11-28T10:00:00.375Z -ref2-seq 10
```

Flags: `-ext` filters by extension, `-pattern` supplies a regex with one
capture group for the sequence number (default: last digit run in the file
name), `-start-seq` overrides the anchor sequence number.

Output lists every frame with its real timestamp, marks each gap, and prints
the first gap's fill candidates with their expected timestamps.
