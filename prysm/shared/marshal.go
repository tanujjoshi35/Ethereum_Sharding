// Package shared defines independent utilities helpful for a sharding-enabled,
// Ethereum blockchain such as blob serialization as more.
package shared

import (
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/rlp"
)

var (
	chunkSize      = int64(32)
	indicatorSize  = int64(1)
	chunkDataSize  = chunkSize - indicatorSize
	skipEvmBits    = byte(0x80)
	dataLengthBits = byte(0x1F)
)

// Flags to add to chunk delimiter.
type Flags struct {
	skipEvmExecution bool
}

// RawBlob type which will contain flags and data for serialization.
type RawBlob struct {
	flags Flags
	data  []byte
}

// NewRawBlob builds a raw blob from any interface by using
// RLP encoding.
func NewRawBlob(i interface{}, skipEvm bool) (*RawBlob, error) {
	data, err := rlp.EncodeToBytes(i)
	if err != nil {
		return nil, fmt.Errorf("RLP encoding was a failure:%v", err)
	}
	return &RawBlob{data: data, flags: Flags{skipEvmExecution: skipEvm}}, nil
}

// ConvertFromRawBlob converts raw blob back from a byte array
// to its interface.
func ConvertFromRawBlob(blob *RawBlob, i interface{}) error {
	data := (*blob).data
	err := rlp.DecodeBytes(data, i)
	if err != nil {
		return fmt.Errorf("RLP decoding was a failure:%v", err)
	}

	return nil
}

// getNumChunks calculates the number of chunks that will be produced by a byte array of given length
func getNumChunks(dataSize int) int {
	numChunks := math.Ceil(float64(dataSize) / float64(chunkDataSize))
	return int(numChunks)
}

// getSerializedDatasize determines the number of bytes that will be produced by a byte array of given length
func getSerializedDatasize(dataSize int) int {
	return getNumChunks(dataSize) * int(chunkSize)
}

// getTerminalLength determines the length of the final chunk for a byte array of given length
func getTerminalLength(dataSize int) int {
	numChunks := getNumChunks(dataSize)
	return dataSize - ((numChunks - 1) * int(chunkDataSize))
}

// Serialize takes a set of blobs and converts them to a single byte array.
func Serialize(rawBlobs []*RawBlob) ([]byte, error) {
	// Loop through all blobs and determine the amount of space that needs to be allocated
	totalDataSize := 0
	for i := 0; i < len(rawBlobs); i++ {
		blobDataSize := len(rawBlobs[i].data)
		totalDataSize += getSerializedDatasize(blobDataSize)
	}

	returnData := make([]byte, 0, totalDataSize)

	// Loop through every blob and copy one chunk at a time
	for i := 0; i < len(rawBlobs); i++ {
		rawBlob := rawBlobs[i]
		numChunks := getNumChunks(len(rawBlob.data))

		for j := 0; j < numChunks; j++ {
			var terminalLength int

			// if non-terminal chunk
			if j != numChunks-1 {
				terminalLength = int(chunkDataSize)

				// append indicating byte with just the length bits
				returnData = append(returnData, byte(0))
			} else {
				terminalLength = getTerminalLength(len(rawBlob.data))

				indicatorByte := byte(terminalLength)
				// include skipEvm flag if true
				if rawBlob.flags.skipEvmExecution {
					indicatorByte = indicatorByte | skipEvmBits
				}

				returnData = append(returnData, indicatorByte)
			}

			// append data bytes
			chunkStart := j * int(chunkDataSize)
			chunkEnd := chunkStart + terminalLength
			blobSlice := rawBlob.data[chunkStart:chunkEnd]
			returnData = append(returnData, blobSlice...)

			// append filler bytes, if necessary
			if terminalLength != int(chunkDataSize) {
				numFillerBytes := numChunks*int(chunkDataSize) - len(rawBlob.data)
				fillerBytes := make([]byte, numFillerBytes)
				returnData = append(returnData, fillerBytes...)
			}
		}
	}

	return returnData, nil
}

// isSkipEvm is true if the first bit is 1
func isSkipEvm(indicator byte) bool {
	return indicator&skipEvmBits>>7 == 1
}

// getDatabyteLength is calculated by looking at the last 5 bits.
// Therefore, mask the first 3 bits to 0
func getDatabyteLength(indicator byte) int {
	return int(indicator & dataLengthBits)
}

// SerializedBlob is a helper struct used by Deserialize to determine the total size of the data byte array
type SerializedBlob struct {
	numNonTerminalChunks int
	terminalLength       int
}

// Deserialize results in the byte array being deserialised and
// separated into its respective interfaces.
func Deserialize(data []byte) ([]RawBlob, error) {
	chunksNumber := len(data) / int(chunkSize)
	serializedBlobs := []SerializedBlob{}
	numPartitions := 0

	// first iterate through every chunk and identify blobs and their length
	for i := 0; i < chunksNumber; i++ {
		indicatorIndex := i * int(chunkSize)
		databyteLength := getDatabyteLength(data[indicatorIndex])

		// if indicator is non-terminal, increase partitions counter
		if databyteLength == 0 {
			numPartitions++
		} else {
			// if indicator is terminal, append blob info and reset partitions counter
			serializedBlob := SerializedBlob{
				numNonTerminalChunks: numPartitions,
				terminalLength:       databyteLength,
			}
			serializedBlobs = append(serializedBlobs, serializedBlob)
			numPartitions = 0
		}
	}

	// for each block, construct the data byte array
	deserializedBlob := make([]RawBlob, 0, len(serializedBlobs))
	currentByte := 0
	for i := 0; i < len(serializedBlobs); i++ {
		numNonTerminalChunks := serializedBlobs[i].numNonTerminalChunks
		terminalLength := serializedBlobs[i].terminalLength

		blob := RawBlob{}
		blob.data = make([]byte, 0, numNonTerminalChunks*31+terminalLength)

		// append data from non-terminal chunks
		for chunk := 0; chunk < numNonTerminalChunks; chunk++ {
			dataBytes := data[currentByte+1 : currentByte+32]
			blob.data = append(blob.data, dataBytes...)
			currentByte += 32
		}

		if isSkipEvm(data[currentByte]) {
			blob.flags.skipEvmExecution = true
		}

		// append data from terminal chunk
		dataBytes := data[currentByte+1 : currentByte+terminalLength+1]
		blob.data = append(blob.data, dataBytes...)
		currentByte += 32

		deserializedBlob = append(deserializedBlob, blob)
	}

	return deserializedBlob, nil
}
