package dsd

import (
	"encoding/binary"
	"fmt"
	"io"
)

func readU16BE(r io.ReaderAt, off int64) (uint16, error) {
	var buf [2]byte
	if err := readFullAt(r, buf[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func readU32BE(r io.ReaderAt, off int64) (uint32, error) {
	var buf [4]byte
	if err := readFullAt(r, buf[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func readU64BE(r io.ReaderAt, off int64) (uint64, error) {
	var buf [8]byte
	if err := readFullAt(r, buf[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}

func readU32LE(r io.ReaderAt, off int64) (uint32, error) {
	var buf [4]byte
	if err := readFullAt(r, buf[:], off); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func readU64LE(r io.ReaderAt, off int64) (uint64, error) {
	var buf [8]byte
	if err := readFullAt(r, buf[:], off); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

func u64ToI64(value uint64, what string) (int64, error) {
	if value > uint64(^uint64(0)>>1) {
		return 0, fmt.Errorf("%s is too large: %d", what, value)
	}
	return int64(value), nil
}

func addInt64(a, b int64, what string) (int64, error) {
	if b > 0 && a > int64(^uint64(0)>>1)-b {
		return 0, fmt.Errorf("%s overflows int64", what)
	}
	if b < 0 && a < -int64(^uint64(0)>>1)-b {
		return 0, fmt.Errorf("%s underflows int64", what)
	}
	return a + b, nil
}

func mulInt64(a, b int64, what string) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("%s has a negative factor", what)
	}
	if a != 0 && b > int64(^uint64(0)>>1)/a {
		return 0, fmt.Errorf("%s overflows int64", what)
	}
	return a * b, nil
}

func readFourCC(r io.ReaderAt, off int64) (string, error) {
	var buf [4]byte
	if err := readFullAt(r, buf[:], off); err != nil {
		return "", err
	}
	return string(buf[:]), nil
}
