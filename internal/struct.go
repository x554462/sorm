package internal

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// error def
var (
	ErrNotSupportStructField = errors.New("struct field not support")
	ErrNotSupportType        = errors.New("data type not support")
	ErrTargetNotSettable     = errors.New("target not settable")
)

type ScanError struct {
	err                   error
	structName, fieldName string
	from, to              reflect.Type
}

func (s ScanError) Error() string {
	return fmt.Sprintf("[scan]: %s.%s is %s which is not assignable by %s: %v", s.structName, s.fieldName, s.to, s.from, s.err)
}

func (s ScanError) Unwrap() error {
	return s.err
}

func newScanError(err error, structName, fieldName string, from, to reflect.Type) ScanError {
	return ScanError{err, structName, fieldName, from, to}
}

type TypeIfe interface {
	BindModel(target interface{})
}

// scanner
func ScanStructSlice(data []map[string]interface{}, target interface{}, tagName string, bindModel bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error:[%v], stack:[%s]", r, string(debug.Stack()))
		}
	}()
	targetValue := reflect.ValueOf(target)
	if !targetValue.Elem().CanSet() {
		return ErrTargetNotSettable
	}
	length := len(data)
	targetValueSlice := reflect.MakeSlice(targetValue.Elem().Type(), 0, length)
	targetValueSliceType := targetValueSlice.Type().Elem()
	for i := 0; i < length; i++ {
		targetObj := reflect.New(targetValueSliceType)
		if err = ScanStruct(data[i], targetObj.Interface(), tagName, bindModel); err != nil {
			return err
		}
		targetValueSlice = reflect.Append(targetValueSlice, targetObj.Elem())
	}
	targetValue.Elem().Set(targetValueSlice)
	return nil
}

func ScanStruct(data map[string]interface{}, target interface{}, tagName string, bindModel bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error:[%v], stack:[%s]", r, string(debug.Stack()))
		}
	}()
	targetType := reflect.TypeOf(target).Elem()
	targetValue := reflect.ValueOf(target).Elem()
	targetValueType := targetValue.Type()
	if targetValueType.Kind() == reflect.Ptr {
		targetValueObj := reflect.New(targetValueType.Elem())
		targetValueObjIfe := targetValueObj.Interface()
		err = ScanStruct(data, targetValueObjIfe, tagName, bindModel)
		if nil == err {
			targetValue.Set(targetValueObj)
		}
		return
	}
	targetName := targetType.Name()
	for i := 0; i < targetType.NumField(); i++ {
		targetTypeField := targetType.Field(i)
		if tagValue, ok := targetTypeField.Tag.Lookup(tagName); ok {
			if idx := strings.IndexByte(tagValue, ','); idx != -1 {
				if tagValue[:idx] == "pk" {
					tagValue = tagValue[idx+1:]
				} else {
					tagValue = tagValue[:idx]
				}
			}
			targetValueField := targetValue.Field(i)
			targetValueFieldIfe := targetValueField.Addr().Interface()
			if dataVal, ok := data[tagValue]; ok {
				if targetValueField.CanSet() {
					if scanner, ok := targetValueFieldIfe.(sql.Scanner); ok {
						err = scanner.Scan(dataVal)
						if bindModel {
							if typer, ok := targetValueFieldIfe.(TypeIfe); ok {
								typer.BindModel(target)
							}
						}
					} else {
						if targetValueField.Kind() == reflect.Ptr {
							targetValueField.Set(reflect.New(targetValueField.Type().Elem()))
							targetValueField = targetValueField.Elem()
						}
						err = scan(targetValueField, dataVal)
					}
				} else {
					err = ErrTargetNotSettable
				}
				if err != nil {
					err = newScanError(err, targetName, targetTypeField.Name, reflect.TypeOf(dataVal), targetValueField.Type())
					return
				}
			} else if bindModel {
				if typer, ok := targetValueFieldIfe.(TypeIfe); ok {
					typer.BindModel(target)
				}
			}
		}
	}
	return err
}

func scan(targetValueField reflect.Value, dataVal interface{}) (err error) {
	kind := reflect.TypeOf(dataVal).Kind()
	switch {
	case isIntSeriesType(kind):
		err = integerConverter(targetValueField, dataVal, false)
	case isUintSeriesType(kind):
		err = integerConverter(targetValueField, dataVal, true)
	case isFloatSeriesType(kind):
		err = floatConverter(targetValueField, dataVal)
	case kind == reflect.Slice:
		err = sliceConverter(targetValueField, dataVal)
	case kind == reflect.String:
		err = stringConverter(targetValueField, dataVal)
	default:
		if dataValTime, ok := dataVal.(time.Time); ok {
			if targetValueField.Kind() == reflect.String {
				targetValueField.SetString(dataValTime.Format("2006-01-02 15:04:05"))
			} else if _, ok := targetValueField.Interface().(time.Time); ok {
				targetValueField.Set(reflect.ValueOf(dataVal))
			} else {
				err = ErrNotSupportType
			}
		} else {
			err = ErrNotSupportType
		}
	}
	return
}

func isIntSeriesType(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Int64
}

func isUintSeriesType(k reflect.Kind) bool {
	return k >= reflect.Uint && k <= reflect.Uint64
}

func isFloatSeriesType(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}

func integerConverter(targetValueField reflect.Value, dataVal interface{}, unsigned bool) error {
	var dataValConverted interface{}
	switch v := dataVal.(type) {
	case int:
		dataValConverted = int64(v)
	case int8:
		dataValConverted = int64(v)
	case int16:
		dataValConverted = int64(v)
	case int32:
		dataValConverted = int64(v)
	case uint:
		dataValConverted = uint64(v)
	case uint8:
		dataValConverted = uint64(v)
	case uint16:
		dataValConverted = uint64(v)
	case uint32:
		dataValConverted = uint64(v)
	case int64, uint64:
		dataValConverted = v
	default:
		return ErrNotSupportType
	}
	if isIntSeriesType(targetValueField.Kind()) {
		if unsigned {
			targetValueField.SetInt(int64(dataValConverted.(uint64)))
		} else {
			targetValueField.SetInt(dataValConverted.(int64))
		}
	} else if isUintSeriesType(targetValueField.Kind()) {
		if unsigned {
			targetValueField.SetUint(dataValConverted.(uint64))
		} else {
			targetValueField.SetUint(uint64(dataValConverted.(int64)))
		}
	} else if targetValueField.Kind() == reflect.Bool {
		if unsigned && dataValConverted.(uint64) > 0 || !unsigned && dataValConverted.(int64) > 0 {
			targetValueField.SetBool(true)
		} else {
			targetValueField.SetBool(false)
		}
	} else if targetValueField.Kind() == reflect.String {
		if unsigned {
			targetValueField.SetString(strconv.FormatUint(dataValConverted.(uint64), 10))
		} else {
			targetValueField.SetString(strconv.FormatInt(dataValConverted.(int64), 10))
		}
	} else if targetValueField.Kind() == reflect.Float64 || targetValueField.Kind() == reflect.Float32 {
		if unsigned {
			targetValueField.SetFloat(float64(dataValConverted.(uint64)))
		} else {
			targetValueField.SetFloat(float64(dataValConverted.(int64)))
		}
	} else {
		return ErrNotSupportStructField
	}
	return nil
}

func floatConverter(targetValueField reflect.Value, dataVal interface{}) error {
	var dataValFloat64 float64
	switch v := dataVal.(type) {
	case float32:
		dataValFloat64 = float64(v)
	case float64:
		dataValFloat64 = v
	default:
		return ErrNotSupportType
	}
	if isIntSeriesType(targetValueField.Kind()) {
		targetValueField.SetInt(int64(dataValFloat64))
	} else if isUintSeriesType(targetValueField.Kind()) {
		targetValueField.SetUint(uint64(dataValFloat64))
	} else if targetValueField.Kind() == reflect.Bool {
		if dataValFloat64 > 0 {
			targetValueField.SetBool(true)
		} else {
			targetValueField.SetBool(false)
		}
	} else if targetValueField.Kind() == reflect.String {
		targetValueField.SetString(strconv.FormatFloat(dataValFloat64, 'f', 6, 64))
	} else if targetValueField.Kind() == reflect.Float64 || targetValueField.Kind() == reflect.Float32 {
		targetValueField.SetFloat(dataValFloat64)
	} else {
		return ErrNotSupportStructField
	}
	return nil
}

func sliceConverter(targetValueField reflect.Value, dataVal interface{}) error {
	dataValSlice, ok := dataVal.([]byte)
	if !ok {
		return ErrNotSupportType
	}
	dataValStr := BytesToString(dataValSlice)
	kind := targetValueField.Kind()
	switch {
	case kind == reflect.String:
		targetValueField.SetString(dataValStr)
	case isIntSeriesType(kind):
		dataValInt, err := strconv.ParseInt(dataValStr, 10, 64)
		if err != nil {
			return err
		}
		targetValueField.SetInt(dataValInt)
	case isUintSeriesType(kind):
		dataValUint, err := strconv.ParseUint(dataValStr, 10, 64)
		if err != nil {
			return err
		}
		targetValueField.SetUint(dataValUint)
	case isFloatSeriesType(kind):
		dataValFloat, err := strconv.ParseFloat(dataValStr, 64)
		if err != nil {
			return err
		}
		targetValueField.SetFloat(dataValFloat)
	case kind == reflect.Bool:
		dataValBool, err := strconv.ParseInt(dataValStr, 10, 64)
		if err != nil {
			return err
		}
		if dataValBool > 0 {
			targetValueField.SetBool(true)
		} else {
			targetValueField.SetBool(false)
		}
	default:
		return ErrNotSupportStructField
	}
	return nil
}

func stringConverter(targetValueField reflect.Value, dataVal interface{}) error {
	dataValStr, ok := dataVal.(string)
	if !ok {
		return ErrNotSupportType
	}
	kind := targetValueField.Kind()
	switch {
	case kind == reflect.String:
		targetValueField.SetString(dataValStr)
	case isIntSeriesType(kind):
		dataValInt, err := strconv.ParseInt(dataValStr, 10, 64)
		if err != nil {
			return err
		}
		targetValueField.SetInt(dataValInt)
	case isUintSeriesType(kind):
		dataValUint, err := strconv.ParseUint(dataValStr, 10, 64)
		if err != nil {
			return err
		}
		targetValueField.SetUint(dataValUint)
	case isFloatSeriesType(kind):
		dataValFloat, err := strconv.ParseFloat(dataValStr, 64)
		if err != nil {
			return err
		}
		targetValueField.SetFloat(dataValFloat)
	case kind == reflect.Bool:
		dataValBool, err := strconv.ParseInt(dataValStr, 10, 64)
		if err != nil {
			return err
		}
		if dataValBool > 0 {
			targetValueField.SetBool(true)
		} else {
			targetValueField.SetBool(false)
		}
	default:
		return ErrNotSupportStructField
	}
	return nil
}
