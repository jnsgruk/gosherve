package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"gopkg.in/check.v1"
)

// This file contains the initial wire up for gocheck and common
// helper functions that are used for testing across the server
// package.

func Test(t *testing.T) { check.TestingT(t) }

func NewMockRedirectSource() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mockRedirects1" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockRedirects1))
			return
		}
		if r.URL.Path == "/mockRedirects2" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockRedirects2))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	}))
}

// readGauge is a helper function for reading prometheus Gauge values
func readGauge(m prometheus.Gauge) float64 {
	pb := &dto.Metric{}
	m.Write(pb)
	return pb.GetGauge().GetValue()
}

// readCounter is a helper function for reading prometheus Counter values
func readCounter(m prometheus.Counter) float64 {
	pb := &dto.Metric{}
	m.Write(pb)
	return *pb.GetCounter().Value
}

// readCounterVec is a helper function for reading prometheus CounterVec values
func readCounterVec(m prometheus.CounterVec, lbl string) float64 {
	pb := &dto.Metric{}
	c, _ := m.GetMetricWithLabelValues(lbl)
	c.Write(pb)
	return *pb.GetCounter().Value
}
