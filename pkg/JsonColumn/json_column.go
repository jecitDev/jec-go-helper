package jsoncolumn

import (
	"database/sql/driver"
	"encoding/json"
)

type JsonColumn[T any] struct {
	v *T
}

func (j *JsonColumn[T]) Scan(src any) error {
	if src == nil {
		j.v = nil
		return nil
	}
	j.v = new(T)
	return json.Unmarshal(src.([]byte), j.v)
}

func (j *JsonColumn[T]) Value() (driver.Value, error) {
	raw, err := json.Marshal(j.v)
	return raw, err
}

func (j *JsonColumn[T]) Get() *T {
	return j.v
}
