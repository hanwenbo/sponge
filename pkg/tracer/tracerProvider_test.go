package tracer

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	exporter, err := newExporter(os.Stdout)
	assert.NoError(t, err)
	resource := NewResource()
	Init(exporter, resource)

	Init(exporter, resource, 0.5)
	Init(exporter, resource, -1.0)
}

func TestClose(t *testing.T) {
	exporter, err := newExporter(os.Stdout)
	assert.NoError(t, err)
	resource := NewResource()
	Init(exporter, resource)
	_ = Close(context.Background())

	tp = nil
	_ = Close(context.Background())
}
