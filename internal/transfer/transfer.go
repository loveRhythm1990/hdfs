// Package transfer implements wire transfer with the datanodes.
package transfer

import (
	"encoding/binary"
	"fmt"
	"io"

	hdfs "github.com/colinmarc/hdfs/v2/internal/protocol/hadoop_hdfs"
	"github.com/golang/protobuf/proto"
)

const (
	dataTransferVersion = 0x1c
	writeBlockOp        = 0x50
	readBlockOp         = 0x51
	checksumBlockOp     = 0x55
)

func makePrefixedMessage(msg proto.Message) ([]byte, error) {
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	lengthBytes := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(lengthBytes, uint64(len(msgBytes)))
	return append(lengthBytes[:n], msgBytes...), nil
}

func readPrefixedMessage(r io.Reader, msg proto.Message) error {
	varintBytes := make([]byte, binary.MaxVarintLen32)
	_, err := io.ReadAtLeast(r, varintBytes, binary.MaxVarintLen32)
	if err != nil {
		return err
	}

	respLength, varintLength := binary.Uvarint(varintBytes)
	if varintLength < 1 {
		return io.ErrUnexpectedEOF
	}

	// We may have grabbed too many bytes when reading the varint.
	respBytes := make([]byte, respLength)
	extraLength := copy(respBytes, varintBytes[varintLength:])
	_, err = io.ReadFull(r, respBytes[extraLength:])
	if err != nil {
		return err
	}

	return proto.Unmarshal(respBytes, msg)
}

// A op request to a datanode:
// +-----------------------------------------------------------+
// |  Data Transfer Protocol Version, int16                    |
// +-----------------------------------------------------------+
// |  Op code, 1 byte                                          |
// +-----------------------------------------------------------+
// |  varint length + OpReadBlockProto                         |
// +-----------------------------------------------------------+
func writeBlockOpRequest(w io.Writer, op uint8, msg proto.Message) error {
	header := []byte{0x00, dataTransferVersion, op}
	msgBytes, err := makePrefixedMessage(msg)
	if err != nil {
		return err
	}

	req := append(header, msgBytes...)
	_, err = w.Write(req)
	if err != nil {
		return err
	}

	return nil
}

// The initial response from a datanode, in the case of reads and writes:
// +-----------------------------------------------------------+
// |  varint length + BlockOpResponseProto                     |
// +-----------------------------------------------------------+
func readBlockOpResponse(r io.Reader) (*hdfs.BlockOpResponseProto, error) {
	resp := &hdfs.BlockOpResponseProto{}
	err := readPrefixedMessage(r, resp)

	return resp, err
}

func getDatanodeAddress(datanode *hdfs.DatanodeIDProto, useHostname bool) string {
	var host string
	if useHostname {
		host = datanode.GetHostName()
	} else {
		host = datanode.GetIpAddr()
	}

	return fmt.Sprintf("%s:%d", host, datanode.GetXferPort())
}