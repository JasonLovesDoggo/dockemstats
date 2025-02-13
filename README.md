# DockemStats

A lightweight tool for simulating and benchmarking Docker registry pulls. Test your registry's performance or just see how many containers you can pull before your network admin notices.

## What is this?

DockemStats is a Go-based utility that simulates concurrent Docker image manifest pulls from popular container registries. It's useful for:

- Testing registry performance under load
- Benchmarking pull rates with different configurations
- Simulating realistic container pull patterns
- Figuring out how your registry performs when everyone in your org decides to deploy at the same time

## Features

- Supports multiple registries (Docker Hub, GitHub Container Registry)
- Simulates realistic client behavior with varied user agents and IPs
- Handles authentication for private registries
- Configurable concurrency and request pacing
- Real-time progress bar and performance metrics
- Won't actually download any containers (just the manifests)

## Installation

```bash
go get github.com/jasonlovesdoggo/dockemstats
```

Or clone and build:

```bash
git clone https://github.com/jasonlovesdoggo/dockemstats.git
cd dockemstats
go build
```

## Usage

```bash
./dockemstats --image nginx:latest --pulls 100 --concurrent 10
```

### Options

- `--image`: Docker image name (e.g., `nginx:latest` or `ghcr.io/username/repo:tag`)
- `--pulls`: Number of pulls to simulate (default: 1)
- `--registry`: Registry to use (`dockerhub` or `ghcr`, default: `dockerhub`)
- `--delay`: Delay between requests in milliseconds (default: 50)
- `--concurrent`: Number of concurrent requests (default: 5)

## Examples

Test Docker Hub with 200 requests at 20 concurrent:
```bash
./dockemstats --image nginx:latest --pulls 200 --concurrent 20 --registry dockerhub
```

Test GitHub Container Registry:
```bash
./dockemstats --image ghcr.io/username/repo:latest --pulls 100 --registry ghcr
```

## Disclaimer

This tool simulates manifest pulls only and is intended for testing and benchmarking purposes. Please use responsibly and in accordance with your registry's terms of service. Don't be that person who DoS's their company's registry during deployment week.

## License

MIT License (see LICENSE file for details)

## Contributing

Pull requests welcome! Just don't add features that would make your DevOps team nervous.