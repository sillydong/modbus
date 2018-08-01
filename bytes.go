package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
)

func Split(data []byte, address uint16, quantity uint16) ([]byte, error) {
	start := int(address) * 2
	stop := start + int(quantity)*2
	if stop > len(data) {
		return nil, fmt.Errorf("Requested address range [%v: %v] out of range [0: %v]", start, stop, len(data))
	}
	return data[start : start+4], nil
}

func Float32ToByte(float float32) []byte {
	bits := math.Float32bits(float)
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, bits)

	return bytes
}

func ByteToFloat32(bytes []byte) float32 {
	bits := binary.BigEndian.Uint32(bytes)

	return math.Float32frombits(bits)
}
