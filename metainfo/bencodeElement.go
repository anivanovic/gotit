package metainfo

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
	return string(bencode.value)
}

type ListElement struct {
	elements []string
}

func (bencode ListElement) Type() string {
	return "List"
}

func (bencode ListElement) GetData() string {
	data := "["
	
	for i := 0; i < len(bencode.elements); i++ {
		data += bencode.elements[i] + ", "
	}
	
	return data[:len(data) - 2] + "]"
}

type DictElement struct {
	dictMap map[string]string
}

func (bencode DictElement) Type() string {
	return "Dictionery"
}

func (bencode DictElement) GetData() string {
	return ""
}




