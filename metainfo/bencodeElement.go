package metainfo

import (
	"fmt"
	"strconv"
)

type bencode interface {
	Type() string
	GetData() string
}

type StringElement struct {
	value string
}

func (bencode StringElement) Type() string {
	return "String"
}

func (bencode StringElement) GetData() string {
	return bencode.value
}

type IntElement struct {
	value int
}

func (bencode IntElement) Type() string {
	return "Integer"
}

func (bencode IntElement) GetData() string {
	return strconv.Itoa(bencode.value)
}

type ListElement struct {
	elements []bencode
}

func (bencode ListElement) Type() string {
	return "List"
}

func (bencode ListElement) GetData() string {
	data := "["

	for i := 0; i < len(bencode.elements); i++ {
		if bencode.elements[i] == nil {
			fmt.Println("element nil: ", i)
		}
		data += bencode.elements[i].GetData() + ", "
	}

	return data[:len(data)-2] + "]"
}

type DictElement struct {
	dict map[StringElement]bencode
}

func (bencode DictElement) Type() string {
	return "Dictionery"
}

func (bencode DictElement) GetData() string {
	return bencode.printValue(bencode, "\t")
}

func (bencode DictElement) printValue(value bencode, tabs string) string {
	dict, ok := value.(DictElement)
	if ok {
		var data = "{\n"
		for k, v := range dict.dict {
			data += tabs + k.GetData() + ": " + bencode.printValue(v, tabs+"\t") + "\n"
		}

		return data + "}\n"
	} else {
		return value.GetData()
	}
}