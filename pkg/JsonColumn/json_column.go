package jsoncolumn

import (
	"database/sql/driver"
	"encoding/json"
)

type JsonColumn[T any] struct {
	V *T
}

func (j *JsonColumn[T]) Scan(src any) error {
	if src == nil {
		j.V = nil
		return nil
	}
	j.V = new(T)
	return json.Unmarshal(src.([]byte), j.V)
}

func (j *JsonColumn[T]) Value() (driver.Value, error) {
	raw, err := json.Marshal(j.V)
	return raw, err
}

func (j *JsonColumn[T]) Get() *T {
	return j.V
}
