package patchtools

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

func PopulateStruct(dataSlice []Data, reg interface{}) error {
	regVal := reflect.ValueOf(reg).Elem()
	regType := regVal.Type()

	// Precompute tag-to-field mapping
	tagFieldMap := make(map[string]int)
	for i := 0; i < regType.NumField(); i++ {
		field := regType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" {
			tagFieldMap[jsonTag] = i
		}
	}

	for _, data := range dataSlice {
		fieldIndex, exists := tagFieldMap[data.Field]
		if !exists {
			continue // Skip unknown fields
		}

		fieldVal := regVal.Field(fieldIndex)
		if fieldVal.IsValid() && fieldVal.CanSet() && fieldVal.Kind() == reflect.Ptr {
			elemType := fieldVal.Type().Elem()
			switch elemType.Kind() {
			case reflect.String:
				strValue := data.Value
				fieldVal.Set(reflect.ValueOf(&strValue))

			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intValue, err := strconv.ParseInt(data.Value, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid int value for field %s: %v", data.Field, err)
				}
				val := reflect.New(elemType).Elem()
				val.SetInt(intValue)
				fieldVal.Set(val.Addr())

			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				uintValue, err := strconv.ParseUint(data.Value, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid uint value for field %s: %v", data.Field, err)
				}
				val := reflect.New(elemType).Elem()
				val.SetUint(uintValue)
				fieldVal.Set(val.Addr())

			case reflect.Float32, reflect.Float64:
				floatValue, err := strconv.ParseFloat(data.Value, 64)
				if err != nil {
					return fmt.Errorf("invalid float value for field %s: %v", data.Field, err)
				}
				val := reflect.New(elemType).Elem()
				val.SetFloat(floatValue)
				fieldVal.Set(val.Addr())

			case reflect.Bool:
				boolValue, err := strconv.ParseBool(data.Value)
				if err != nil {
					return fmt.Errorf("invalid bool value for field %s: %v", data.Field, err)
				}
				val := reflect.New(elemType).Elem()
				val.SetBool(boolValue)
				fieldVal.Set(val.Addr())

			case reflect.Struct:
				if elemType == reflect.TypeOf(time.Time{}) {
					timeValue, err := time.Parse(time.RFC3339, data.Value)
					if err != nil {
						return fmt.Errorf("invalid time value for field %s: %v", data.Field, err)
					}
					fieldVal.Set(reflect.ValueOf(&timeValue))
				} else {
					return fmt.Errorf("unsupported struct type for field %s", data.Field)
				}

			default:
				return fmt.Errorf("unsupported field type: %s", elemType.Kind())
			}
		}
	}

	return nil
}
