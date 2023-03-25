package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	var (
		flagAddress = flag.String("address", "localhost:8888", "Listen address")
	)

	flag.Parse()

	if flag.NArg() == 0 {
		log.Println("No devices specified.")
		os.Exit(1)
	}

	devices, err := parseDevices(flag.Args())
	if err != nil {
		log.Printf("Error parsing devices: %s", err)
		os.Exit(1)
	}

	deviceAddrs := make(map[string]string)
	for name, addr := range devices {
		deviceAddrs[name] = addr
	}

	client := http.Client{Timeout: 2 * time.Second}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(newCollector(client, deviceAddrs))

	log.Printf("Awair exporter listening on %s", *flagAddress)
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	log.Fatal(http.ListenAndServe(*flagAddress, nil))
}

// parseDevices parses a list of "key=value" strings into a map[key]value.
func parseDevices(args []string) (map[string]string, error) {
	devices := make(map[string]string)

	for _, arg := range args {
		index := strings.LastIndex(arg, "=")
		if index < 0 {
			return nil, fmt.Errorf("expected key=value, got %q", arg)
		}
		devices[arg[:index]] = arg[index+1:]
	}

	return devices, nil
}

type collector struct {
	Client http.Client

	// DeviceAddrs maps a readable device name to its scrape addr
	DeviceAddrs map[string]string

	Errors         *prometheus.Desc
	Score          *prometheus.Desc
	DewPointC      *prometheus.Desc
	DewPointF      *prometheus.Desc
	TempC          *prometheus.Desc
	TempF          *prometheus.Desc
	Humid          *prometheus.Desc
	AbsHumid       *prometheus.Desc
	Co2            *prometheus.Desc
	Co2Est         *prometheus.Desc
	Co2EstBaseline *prometheus.Desc
	Voc            *prometheus.Desc
	VocBaseline    *prometheus.Desc
	VocH2Raw       *prometheus.Desc
	VocEthanolRaw  *prometheus.Desc
	Pm25           *prometheus.Desc
	Pm10Est        *prometheus.Desc
}

func newCollector(client http.Client, deviceAddrs map[string]string) *collector {
	return &collector{
		Client:      client,
		DeviceAddrs: deviceAddrs,

		Errors: prometheus.NewDesc(
			"awair_collection_errors_total",
			"Errors observed when collecting device metrics",
			[]string{"sensor"},
			nil,
		),

		Score: prometheus.NewDesc(
			"awair_score",
			"Awair Score (0-100)",
			[]string{"sensor"},
			nil,
		),

		DewPointC: prometheus.NewDesc(
			"awair_dew_point",
			"The temperature at which water will condense and form into dew (C)",
			[]string{"sensor"},
			nil,
		),

		DewPointF: prometheus.NewDesc(
			"awair_dew_point_f",
			"The temperature at which water will condense and form into dew (F)",
			[]string{"sensor"},
			nil,
		),

		TempC: prometheus.NewDesc(
			"awair_temp",
			"Dry bulb temperature (C)",
			[]string{"sensor"},
			nil,
		),

		TempF: prometheus.NewDesc(
			"awair_temp_f",
			"Dry bulb temperature (F)",
			[]string{"sensor"},
			nil,
		),

		Humid: prometheus.NewDesc(
			"awair_humid",
			"Relative humidity (%)",
			[]string{"sensor"},
			nil,
		),

		AbsHumid: prometheus.NewDesc(
			"awair_abs_humid",
			"Absolute humidity (g/m^3)",
			[]string{"sensor"},
			nil,
		),

		Co2: prometheus.NewDesc(
			"awair_co2",
			"Carbon Dioxide (ppm)",
			[]string{"sensor"},
			nil,
		),

		Co2Est: prometheus.NewDesc(
			"awair_co2_est",
			"Estimated Carbon Dioxide calculated by TVOC sensor (ppm)",
			[]string{"sensor"},
			nil,
		),

		Co2EstBaseline: prometheus.NewDesc(
			"awair_co2_est_baseline",
			"A unitless value that represents the baseline from which the TVOC sensor partially derives its estimate",
			[]string{"sensor"},
			nil,
		),

		Voc: prometheus.NewDesc(
			"awair_voc",
			"Total Volatile organic compounds (ppb)",
			[]string{"sensor"},
			nil,
		),

		VocBaseline: prometheus.NewDesc(
			"awair_voc_baseline",
			"A unitless value that represents the baseline from which the TVOC sensor partially derives its TVOC output",
			[]string{"sensor"},
			nil,
		),

		VocH2Raw: prometheus.NewDesc(
			"awair_voc_h2_raw",
			"A unitless value that represents the Hydrogen gas signal from which the TVOC sensor partially derives its TVOC output",
			[]string{"sensor"},
			nil,
		),

		VocEthanolRaw: prometheus.NewDesc(
			"awair_voc_ethanol_raw",
			"A unitless value that represents the Ethanol gas signal from which the TVOC sensor partially derives its TVOC output",
			[]string{"sensor"},
			nil,
		),

		Pm25: prometheus.NewDesc(
			"awair_pm25",
			"Particulate matter less than 2.5 microns in diameter (µg/m³)",
			[]string{"sensor"},
			nil,
		),

		Pm10Est: prometheus.NewDesc(
			"awair_pm10_est",
			"Estimated particulate matter less than 10 microns in diameter (µg/m³ - calculated by the PM2.5 sensor)",
			[]string{"sensor"},
			nil,
		),
	}
}

// Describe implements Prometheus.Collector.
func (c collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.Errors
	ch <- c.Score
	ch <- c.DewPointC
	ch <- c.DewPointF
	ch <- c.TempC
	ch <- c.TempF
	ch <- c.Humid
	ch <- c.AbsHumid
	ch <- c.Co2
	ch <- c.Co2Est
	ch <- c.Co2EstBaseline
	ch <- c.Voc
	ch <- c.VocBaseline
	ch <- c.VocH2Raw
	ch <- c.VocEthanolRaw
	ch <- c.Pm25
	ch <- c.Pm10Est
}

// Collect implements Prometheus.Collector.
func (c collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	wg.Add(len(c.DeviceAddrs))

	for name, addr := range c.DeviceAddrs {
		go func(name, addr string) {
			c.collectOne(ch, name, addr)
			wg.Done()
		}(name, addr)
	}

	wg.Wait()
}

func (c collector) collectOne(ch chan<- prometheus.Metric, name, addr string) {
	url := "http://" + addr + "/air-data/latest"

	resp, err := c.Client.Get(url)
	if err != nil {
		log.Printf("[%s] request failed: %v", addr, err)
		return
	}
	defer resp.Body.Close()

	labels := []string{name}

	if resp.StatusCode != 200 {
		log.Printf("[%s:%s] non-200 response: %s", name, addr, resp.Status)
		ch <- prometheus.MustNewConstMetric(c.Errors, prometheus.CounterValue, 1, labels...)
		return
	}

	var airData struct {
		// Timestamp is RFC3339 w/ millis, "2006-01-02T15:04:05.000Z"
		Timestamp      string  `json:"timestamp"`
		Score          int     `json:"score"`
		DewPoint       float64 `json:"dew_point"`
		Temp           float64 `json:"temp"`
		Humid          float64 `json:"humid"`
		AbsHumid       float64 `json:"abs_humid"`
		Co2            int     `json:"co2"`
		Co2Est         int     `json:"co2_est"`
		Co2EstBaseline int     `json:"co2_est_baseline"`
		Voc            int     `json:"voc"`
		VocBaseline    int     `json:"voc_baseline"`
		VocH2Raw       int     `json:"voc_h2_raw"`
		VocEthanolRaw  int     `json:"voc_ethanol_raw"`
		Pm25           int     `json:"pm25"`
		Pm10Est        int     `json:"pm10_est"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&airData); err != nil {
		log.Printf("[%s:%s] could not parse AirData: %s", name, addr, err)
		ch <- prometheus.MustNewConstMetric(c.Errors, prometheus.CounterValue, 1, name)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.Score, prometheus.GaugeValue, float64(airData.Score), labels...)
	ch <- prometheus.MustNewConstMetric(c.DewPointC, prometheus.GaugeValue, airData.DewPoint, labels...)
	ch <- prometheus.MustNewConstMetric(c.DewPointF, prometheus.GaugeValue, celsiusToFahrenheit(airData.DewPoint), labels...)
	ch <- prometheus.MustNewConstMetric(c.TempC, prometheus.GaugeValue, airData.Temp, labels...)
	ch <- prometheus.MustNewConstMetric(c.TempF, prometheus.GaugeValue, celsiusToFahrenheit(airData.Temp), labels...)
	ch <- prometheus.MustNewConstMetric(c.Humid, prometheus.GaugeValue, float64(airData.Humid), labels...)
	ch <- prometheus.MustNewConstMetric(c.AbsHumid, prometheus.GaugeValue, float64(airData.AbsHumid), labels...)
	ch <- prometheus.MustNewConstMetric(c.Co2, prometheus.GaugeValue, float64(airData.Co2), labels...)
	ch <- prometheus.MustNewConstMetric(c.Co2Est, prometheus.GaugeValue, float64(airData.Co2Est), labels...)
	ch <- prometheus.MustNewConstMetric(c.Co2EstBaseline, prometheus.GaugeValue, float64(airData.Co2EstBaseline), labels...)
	ch <- prometheus.MustNewConstMetric(c.Voc, prometheus.GaugeValue, float64(airData.Voc), labels...)
	ch <- prometheus.MustNewConstMetric(c.VocBaseline, prometheus.GaugeValue, float64(airData.VocBaseline), labels...)
	ch <- prometheus.MustNewConstMetric(c.VocEthanolRaw, prometheus.GaugeValue, float64(airData.VocEthanolRaw), labels...)
	ch <- prometheus.MustNewConstMetric(c.VocH2Raw, prometheus.GaugeValue, float64(airData.VocH2Raw), labels...)
	ch <- prometheus.MustNewConstMetric(c.Pm25, prometheus.GaugeValue, float64(airData.Pm25), labels...)
	ch <- prometheus.MustNewConstMetric(c.Pm10Est, prometheus.GaugeValue, float64(airData.Pm10Est), labels...)
}

func celsiusToFahrenheit(tempC float64) float64 {
	return tempC*9/5 + 32
}
