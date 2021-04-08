package scalers

import (
	"context"
	"encoding/json"
	"fmt"
	kedautil "github.com/kedacore/keda/v2/pkg/util"
	"io/ioutil"
	v2beta2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
	"net/http"
	"net/url"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
)

type weatherScaler struct {
	metadata   *weatherMetadata
	httpClient *http.Client
}

type weatherMetadata struct {
	thresholdValue int64
	host           string
	cityName       string
	apiKey         string
	preference     string
}

type WeatherData struct {
	Main struct {
		Temp_min float64 `json:"temp_min"`
		Temp_max float64 `json:"temp_max"`
		Temp     float64 `json:"temp"`
	} `json:"main"`
}

var weatherLog = logf.Log.WithName("weather_scaler")

func NewWeatherScaler(config *ScalerConfig) (Scaler, error) {

	httpClient := kedautil.CreateHTTPClient(config.GlobalHTTPTimeout)

	weatherMetadata, err := ParseWeatherMetadata(config)
	if err != nil {
		return nil, fmt.Errorf("Error parsing weather metadata: %s", err)
	}

	return &weatherScaler{
		metadata:   weatherMetadata,
		httpClient: httpClient,
	}, nil
}

func ParseWeatherMetadata(config *ScalerConfig) (*weatherMetadata, error) {

	meta := weatherMetadata{}

	if val, ok := config.TriggerMetadata["thresholdValue"]; ok && val != "" {
		thresholdValue, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("Error parsing threshold value: %s", err.Error())
		}
		meta.thresholdValue = int64(thresholdValue)
	}

	if config.TriggerMetadata["cityName"] == "" {
		return nil, fmt.Errorf("No city name given")
	}
	meta.cityName = config.TriggerMetadata["cityName"]

	if config.TriggerMetadata["apiKey"] == "" {
		return nil, fmt.Errorf("No API key given")
	}
	meta.apiKey = config.TriggerMetadata["apiKey"]

	if val, ok := config.TriggerMetadata["host"]; ok {
		urlString := fmt.Sprintf(val, meta.cityName, meta.apiKey)
		_, err := url.ParseRequestURI(urlString)
		if err != nil {
			return nil, fmt.Errorf("Invalid URL: %s", err.Error())
		}
		meta.host = string(urlString)
	} else {
		return nil, fmt.Errorf("No host URI given")
	}

	if config.TriggerMetadata["preference"] == "" {
		return nil, fmt.Errorf("No preference given")
	}
	meta.preference = config.TriggerMetadata["preference"]

	return &meta, nil
}

func (s *weatherScaler) IsActive(ctx context.Context) (bool, error) {

	tmpr, err := s.GetWeather()
	if err != nil {
		weatherLog.Error(err, "IsActive function error")
		return false, err
	}

	return int64(tmpr) > s.metadata.thresholdValue, nil
}

func (s *weatherScaler) GetMetricSpecForScaling() []v2beta2.MetricSpec {

	targetMetricValue := resource.NewQuantity(int64(s.metadata.thresholdValue), resource.DecimalSI)
	externalMetric := &v2beta2.ExternalMetricSource{
		Metric: v2beta2.MetricIdentifier{
			Name: "weather",
		},
		Target: v2beta2.MetricTarget{
			Type:         v2beta2.AverageValueMetricType,
			AverageValue: targetMetricValue,
		},
	}
	metricSpec := v2beta2.MetricSpec{External: externalMetric, Type: externalMetricType}
	return []v2beta2.MetricSpec{metricSpec}
}

func (s *weatherScaler) GetMetrics(ctx context.Context, metricName string, metricSelector labels.Selector) ([]external_metrics.ExternalMetricValue, error) {

	tmpr, err := s.GetWeather()
	if err != nil {
		weatherLog.Error(err, "Unable to get GetWeather()")
		return []external_metrics.ExternalMetricValue{}, err
	}

	metric := external_metrics.ExternalMetricValue{
		MetricName: metricName,
		Value:      *resource.NewQuantity(int64(tmpr), resource.DecimalSI),
		Timestamp:  metav1.Now(),
	}

	return append([]external_metrics.ExternalMetricValue{}, metric), nil
}

func (s *weatherScaler) Close() error {
	return nil
}

func (s *weatherScaler) GetJSONData() ([]byte, error) {

	res, err := s.httpClient.Get(s.metadata.host)
	if err != nil {
		weatherLog.Error(err, "Error getting JSON Data")
	}

	jsonBlob, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		weatherLog.Error(err, "Error in JsonBlob")
	}
	return jsonBlob, err
}

func (s *weatherScaler) GetWeather() (int, error) {

	var tmpr int

	var data WeatherData
	jsonBlob, err := s.GetJSONData()

	if err != nil {
		return 100, err
	}

	json.Unmarshal(jsonBlob, &data)

	switch s.metadata.preference {
	case "Temp_min":
		tmpr = int(data.Main.Temp_min)
	case "Temp_max":
		tmpr = int(data.Main.Temp_max)
	case "Temp":
		tmpr = int(data.Main.Temp)
	}

	return tmpr, nil
}
