# keda-weather_scaler
This is a KEDA scaler which takes JSON data from a weather API, which is OpenWeather, and scales the pods up and down by comparing the threshold value I specified in the YAML file and the temperature value that the API returns at that moment.

It is not a real-purpose scaler, was written for the sake of understanding the scaler concepts to be able to contribute to KEDA with a real-purpose scaler.

 
