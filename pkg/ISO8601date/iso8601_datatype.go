package iso8601date

import (
	"encoding/json"
	"errors"
	"regexp"
)

// type ISO8601date ISO8601dateData

type ISO8601date struct {
	datetime string
}

func (c ISO8601date) String() string {
	return string(c.datetime)
}
func Parse(s string) (ISO8601date, error) {
	ISO8601DateRegexString := `^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})([+-])(\d{2}):(\d{2})$`
	ISO8601DateRegex := regexp.MustCompile(ISO8601DateRegexString)

	if ISO8601DateRegex.MatchString(s) {
		return ISO8601date{s}, nil
	}
	return ISO8601date{}, errors.New("invalid iso8601 date format")

}
func (c ISO8601date) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.datetime)
}

func (c *ISO8601date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*c = ISO8601date{
		datetime: s,
	}
	return nil
}
