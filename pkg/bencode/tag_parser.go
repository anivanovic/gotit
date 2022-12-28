package bencode

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
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
		"bencode: field mapped to bencode element of wrong type: element name: %s field type: %s, bencode type: %s",
		e.ElementName, e.FieldType, e.BencodeType)
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

	ttype := val.Type()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		ftype := ttype.Field(i)
		tag := ftype.Tag

		if !f.CanSet() || toBool(tag.Get("ignored")) {
			continue
		}

		name := tag.Get("ben")
		if name == "" {
			name = ftype.Name
		}
		dict, ok := bencode.(*DictElement)
		if !ok {
			return &TypeError{
				ElementName: name,
				FieldType:   f.Type().Name(),
				BencodeType: reflect.TypeOf(dict).String(),
			}
		}
		value := dict.Value(name)
		if value != nil {
			err := setField(f, value)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func setField(f reflect.Value, value Bencode) error {
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
	case reflect.Bool:
		f.SetBool(toBool(value.String()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		vali, err := strconv.ParseInt(value.String(), 0, ftype.Bits())
		if err != nil {
			return err
		}
		f.SetInt(vali)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		valu, err := strconv.ParseUint(value.String(), 0, ftype.Bits())
		if err != nil {
			return err
		}
		f.SetUint(valu)
	case reflect.Float32, reflect.Float64:
		valf, err := strconv.ParseFloat(value.String(), ftype.Bits())
		if err != nil {
			return err
		}
		f.SetFloat(valf)
	case reflect.Slice:
		values, ok := value.(*ListElement)
		if !ok {
			return &TypeError{
				ElementName: "",
				FieldType:   f.Type().Name(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}
		slice := reflect.MakeSlice(ftype, len(values.Value), len(values.Value))

		for i, v := range values.Value {
			listTarget := slice.Index(i)
			err := setField(listTarget, v)
			if err != nil {
				return err
			}
		}

		f.Set(slice)
	case reflect.Struct:
		structPtr := f.Addr().Interface()

		subDict, ok := value.(*DictElement)
		if !ok {
			return &TypeError{
				ElementName: "",
				FieldType:   f.Type().Name(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}
		vType := reflect.TypeOf(value)
		if vType.Kind() == reflect.Ptr {
			vType = vType.Elem()
		}
		if f.Type().Name() == vType.Name() {
			f.Set(reflect.ValueOf(value).Elem())
		}
		err := processTarget(structPtr, subDict)
		if err != nil {
			return err
		}
	case reflect.Map:
		m := reflect.MakeMap(ftype)
		values, ok := value.(*DictElement)
		if !ok {
			return &TypeError{
				ElementName: "",
				FieldType:   f.Type().Name(),
				BencodeType: reflect.TypeOf(value).String(),
			}
		}

		for k, v := range values.value {
			m.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
		}
		f.Set(m)
	}

	return nil
}

func toBool(val string) bool {
	b, _ := strconv.ParseBool(val)
	return b
}
