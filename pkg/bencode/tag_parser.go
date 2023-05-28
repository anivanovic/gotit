package bencode

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
)

var (
	ErrWrongTarget = errors.New("bencode: pointer to struct expected as target")
)

type TypeError struct {
	ElementName string
	FieldType   string
	BencodeType string
}

func (e TypeError) Error() string {
	return fmt.Sprintf(
		"could not assign bencode value to target (field name: %s, field type: %s, bencode type: %s)",
		e.ElementName,
		e.FieldType,
		e.BencodeType,
	)
}

func Unmarshal(data []byte, target interface{}) error {
	ben, err := Parse(data)
	if err != nil {
		return err
	}

	return processTarget(target, ben)
}

func processTarget(target interface{}, bencode Bencode) error {
	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Ptr {
		return ErrWrongTarget
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return ErrWrongTarget
	}

	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		ftype := val.Type().Field(i)
		tagName := ftype.Tag.Get("ben")

		if !f.CanSet() || tagName == "" {
			continue
		}

		dict, ok := bencode.(*DictElement)
		if !ok {
			panic("bencode internal: bencode element not *DictElement")
		}
		value := dict.Value(tagName)
		if value != nil {
			err := setField(f, value, ftype.Name)
			if err != nil {
				return err
			}
		} else {
			// TODO: we should throw error if configured in struct tag
		}
	}

	return nil
}

func setField(f reflect.Value, value Bencode, fieldName string) error {
	ftype := f.Type()

	if ftype.Kind() == reflect.Ptr {
		ftype = ftype.Elem()
		if f.IsNil() {
			f.Set(reflect.New(ftype))
		}

		f = f.Elem()
	}

	switch ftype.Kind() {
	case reflect.String:
		f.SetString(value.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		vali, err := strconv.ParseInt(value.String(), 0, ftype.Bits())
		if err != nil {
			return &TypeError{
				ElementName: fieldName,
				FieldType:   f.Type().String(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}
		f.SetInt(vali)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		valu, err := strconv.ParseUint(value.String(), 0, ftype.Bits())
		if err != nil {
			return err
		}
		f.SetUint(valu)
	case reflect.Slice:
		// support byte array and set to raw value
		if f.Type().Elem().Kind() == reflect.Uint8 {
			f.Set(reflect.ValueOf(value.Raw()))
			return nil
		}

		values, ok := value.(*ListElement)
		if !ok {
			return &TypeError{
				ElementName: fieldName,
				FieldType:   f.Type().String(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}
		slice := reflect.MakeSlice(ftype, len(values.Value), len(values.Value))

		for i, v := range values.Value {
			listTarget := slice.Index(i)
			if err := setField(listTarget, v, fieldName); err != nil {
				return err
			}
		}

		f.Set(slice)
	case reflect.Struct:
		subDict, ok := value.(*DictElement)
		if !ok {
			return &TypeError{
				ElementName: fieldName,
				FieldType:   f.Type().String(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}
		structPtr := f.Addr().Interface()
		if err := processTarget(structPtr, subDict); err != nil {
			return err
		}
	case reflect.Map:
		values, ok := value.(*DictElement)
		if !ok {
			return &TypeError{
				ElementName: fieldName,
				FieldType:   f.Type().String(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}

		m := reflect.MakeMap(ftype)
		for k, v := range values.value {
			mapValue := reflect.New(ftype.Elem())
			if mapValue.Kind() == reflect.Ptr {
				mapValue = mapValue.Elem()
			}
			if err := setField(mapValue, v, fieldName); err != nil {
				return err
			}
			m.SetMapIndex(reflect.ValueOf(k), mapValue)
		}
		f.Set(m)
	default:
		return fmt.Errorf("unsupported bencode type %s", ftype)
	}

	return nil
}

type TorrentFile struct {
	Path   []string `ben:"path"`
	Length int      `ben:"length"`
}

func (f TorrentFile) String() string {
	return fmt.Sprintf("[path: %s, size: %s]", f.FilePath(), bytefmt.ByteSize(uint64(f.Length)))
}

func (f TorrentFile) FilePath() string {
	return strings.Join(f.Path, "/")
}

type Metainfo struct {
	Announce     string     `ben:"announce"`
	AnnounceList [][]string `ben:"announce-list"`
	UrlList      []string   `ben:"url-list"`
	Info         struct {
		Files       []TorrentFile `ben:"files"`
		Length      int64         `ben:"length"`
		Name        string        `ben:"name"`
		PieceLength int64         `ben:"piece length"`
		Pieces      string        `ben:"pieces"`
	} `ben:"info"`
	InfoDictRaw  []byte `ben:"info"`
	Comment      string `ben:"comment"`
	CreatedBy    string `ben:"created by"`
	CreationDate int64  `ben:"creation date"`
}

func (m Metainfo) String() string {
	b := &strings.Builder{}

	b.WriteString("{\n")
	b.WriteString("\tinfo: {\n")
	printValue("name", m.Info.Name, 2, b)
	printValue("length", bytefmt.ByteSize(uint64(m.Info.Length)), 2, b)
	printValue("piece-length", m.Info.PieceLength, 2, b)
	printValue("files", m.Info.Files, 2, b)
	// TODO: should we print pieces?
	b.WriteString("\t}\n")
	printValue("comment", m.Comment, 1, b)
	printValue("created-by", m.CreatedBy, 1, b)
	printValue("creation-date", time.Unix(m.CreationDate, 0).Format(time.DateTime), 1, b)
	printValue("announce", m.Announce, 1, b)
	printValue("announce-list", m.AnnounceList, 1, b)
	printValue("url-list", m.UrlList, 1, b)
	printValue("(calculated) info-hash", base64.URLEncoding.EncodeToString(m.Hash()), 1, b)
	b.WriteString("}\n")

	return b.String()
}

func printValue(name string, value interface{}, ident int, b *strings.Builder) {
	switch value := value.(type) {
	case string:
		if value != "" {
			b.WriteString(strings.Repeat("\t", ident) + name + ": " + value + "\n")
		}
	case int64:
		if value != 0 {
			strVal := strconv.FormatInt(value, 10)
			b.WriteString(strings.Repeat("\t", ident) + name + ": " + strVal + "\n")
		}
	case []string:
		if len(value) != 0 {
			b.WriteString(strings.Repeat("\t", ident) + name + ": [\n")
			for _, s := range value {
				b.WriteString(strings.Repeat("\t", ident+1) + s + ",\n")
			}
			b.WriteString(strings.Repeat("\t", ident) + "]\n")
		}
	case [][]string:
		if len(value) != 0 {
			b.WriteString(strings.Repeat("\t", ident) + name + ": [\n")
			for _, slist := range value {
				b.WriteString(strings.Repeat("\t", ident+1) + fmt.Sprintf("%s", slist) + "\n")
			}
			b.WriteString(strings.Repeat("\t", ident) + "]\n")
		}
	case []TorrentFile:
		if len(value) != 0 {
			b.WriteString(strings.Repeat("\t", ident) + name + ": [\n")
			for _, f := range value {
				b.WriteString(strings.Repeat("\t", ident+1) + f.String() + ",\n")
			}
			b.WriteString(strings.Repeat("\t", ident) + "]\n")
		}
	}
}

func (m Metainfo) Hash() []byte {
	sha := sha1.New()
	sha.Write(m.InfoDictRaw)
	return sha.Sum(nil)
}
