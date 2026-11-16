package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	domain "github.com/zhelnovape/template/internal/domain/trip"
)

type Object map[string]json.RawMessage

func TripID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "tripId"))
	if err != nil {
		return uuid.Nil, &domain.ValidationError{Field: "tripId", Reason: "must be a valid UUID"}
	}
	return id, nil
}

func DecodeJSON(body io.Reader, dst any) (Object, error) {
	invalidBody := &domain.ValidationError{Field: "body", Reason: "must be a valid JSON object"}

	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, invalidBody
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return nil, invalidBody
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, invalidBody
	}

	var obj Object
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, invalidBody
	}
	return obj, nil
}

func (o Object) Nested(field string) (Object, error) {
	var nested Object
	if err := json.Unmarshal(o[field], &nested); err != nil || nested == nil {
		return nil, &domain.ValidationError{Field: field, Reason: "must be an object"}
	}
	return nested, nil
}

func (o Object) Require(prefix string, fields ...string) error {
	for _, f := range fields {
		if v, ok := o[f]; !ok || string(v) == "null" {
			return &domain.ValidationError{Field: prefix + f, Reason: "is required"}
		}
	}
	if len(o) == len(fields) {
		return nil
	}
	for key := range o {
		if !slices.Contains(fields, key) {
			return &domain.ValidationError{Field: prefix + key, Reason: "is not allowed"}
		}
	}
	return nil
}
