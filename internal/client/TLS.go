package client

import (
	"encoding/binary"
	"github.com/cbeuw/Cloak/internal/util"
)

type Browser interface {
	composeClientHello(*State) ([]byte, []byte)
}

func makeServerName(serverName string) []byte {
	serverNameListLength := make([]byte, 2)
	binary.BigEndian.PutUint16(serverNameListLength, uint16(len(serverName)+3))
	serverNameType := []byte{0x00} // host_name
	serverNameLength := make([]byte, 2)
	binary.BigEndian.PutUint16(serverNameLength, uint16(len(serverName)))
	ret := make([]byte, 2+1+2+len(serverName))
	copy(ret[0:2], serverNameListLength)
	copy(ret[2:3], serverNameType)
	copy(ret[3:5], serverNameLength)
	copy(ret[5:], serverName)
	return ret
}

// addExtensionRecord, add type, length to extension data
func addExtRec(typ []byte, data []byte) []byte {
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(data)))
	ret := make([]byte, 2+2+len(data))
	copy(ret[0:2], typ)
	copy(ret[2:4], length)
	copy(ret[4:], data)
	return ret
}

// ComposeClientHello composes ClientHello with record layer
func ComposeClientHello(sta *State) ([]byte, []byte) {
	ch, sharedSecret := sta.Browser.composeClientHello(sta)
	return util.AddRecordLayer(ch, []byte{0x16}, []byte{0x03, 0x01}), sharedSecret
}