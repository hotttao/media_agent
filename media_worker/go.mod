module media_agent/media_worker

go 1.26.3

require media_agent/hertz_infra v0.0.0

require (
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	golang.org/x/sys v0.41.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	media_agent/hertz_gen v0.0.0 // indirect
)

replace (
	media_agent/hertz_gen => ../hertz_gen
	media_agent/hertz_infra => ../hertz_infra
)
