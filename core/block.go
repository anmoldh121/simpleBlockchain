package blockchain

import (
	"time"
)

type Block struct {
	Timestamp int64
	Hash      []byte
	PrevHash  []byte
}

func CreateBlock(_prevHash []byte) *Block {
	return &Block{
		time.Now().Unix(),
		[]byte{},
		_prevHash,
	}
}

func Genesis() *Block {
	return CreateBlock([]byte{})
}
