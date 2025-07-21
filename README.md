# goTApaper: Automated Desktop Wallpaper Manager with Multi-Source Support

goTApaper is a Go application that automatically downloads and sets desktop wallpapers from various providers like Bing,
National Geographic, Unsplash, and Pexels. It features a system tray interface, watermark support, and cross-platform
compatibility for Windows, macOS, and Linux.

The application provides advanced features including configurable refresh intervals, image cropping, watermark
customization, and support for multiple wallpaper sources with probability-based selection. It integrates seamlessly
with system startup and supports proxy configurations for network access. The modular architecture allows easy addition
of new wallpaper sources and desktop environment support.

## Repository Structure

```
.
├── actor/                  # Core functionality for image processing and wallpaper setting
│   ├── crop.go            # Image cropping functionality
│   ├── setter/            # Platform-specific wallpaper setters
│   └── watermark/         # Watermark rendering and font management
├── channel/               # Wallpaper source providers (Bing, Unsplash, etc.)
├── cmd/                   # Command-line interface implementation
├── config/               # Configuration management and defaults
├── contrib/              # Build and packaging resources
├── data/                 # Asset management
├── examples/             # Example configurations and assets
├── generate/             # Code generation utilities
├── history/             # Download history tracking
├── install/             # Installation and auto-start functionality
├── util/                # Common utilities
├── go.mod               # Go module definition
├── Makefile            # Build automation
└── README.md           # Project documentation
```

## Usage Instructions

### Installation

#### From GitHub Release

https://github.com/genzj/goTApaper/releases/latest

#### From Source

```bash
# Clone the repository
git clone https://github.com/genzj/goTApaper
cd goTApaper

# Build for your platform
```

##### Linux

```bash
# Install dependencies
sudo apt-get update
sudo apt-get install build-essential libgtk-3-dev libayatana-appindicator3-dev

# Build
make build-os-linux-amd64
```

##### macOS

```bash
# Install Xcode Command Line Tools
xcode-select --install

# Install create-dmg for packaging
brew install create-dmg

# Build
make build-os-darwin-amd64  # For Intel Macs
make build-os-darwin-arm64  # For Apple Silicon
```

##### Windows

```powershell
# Build
make build-os-windows
```

### Quick Start

1. Create a configuration file:
    ```bash
    ./goTApaper generate-config --install
    ```
2. Edit ~/.goTApaper/config.yaml to configure your preferred wallpaper sources
3. Run in daemon mode:
    ```bash
    ./goTApaper daemon
    ```

### More Detailed Examples

1. Using Bing as wallpaper source:
    ```yaml
    active-channels:
      - bing

    channels:
      bing:
        type: bing-wallpaper
        strategy: largest-no-logo
    ```

2. Adding watermark to wallpapers:

    ```yaml
    watermark:
      - font: NotoSans-Regular.ttf
        point: 13
        color: 222222
        position: bottom-center
        template: |
          {{.Title}} ({{.Credit}} | {{.ChannelKey}})
          {{.UploadTime.Format "2006 Jan 2 15:04:05"}}
    ```

### Troubleshooting

#### Common Issues

1. Font not found

    ```
    Error: Cannot find font file NotoSans-Regular.ttf
    Solution: Specify full path to font file or install font in system directory
    ```

2. Network Access Issues

    ```
    Error: Failed to download wallpaper
    Solution: Configure proxy in config.yaml:
    proxy: socks5://127.0.0.1:1080
    ```

#### Debug Mode

Enable debug logging:

```bash
./goTApaper daemon --debug
```

Debug logs are written to stdout with full timestamps.

## Development

### Environment

- Go 1.23.0 or later
- Git for version control
- Platform-specific build tools:
  - Windows: Visual Studio with C++ tools or MinGW
  - Linux: build-essential, libgtk-3-dev, libayatana-appindicator3-dev
  - macOS: Xcode Command Line Tools

### Data Flow

goTApaper follows a pipeline pattern for wallpaper processing: source selection → download → processing → setting as
wallpaper.

```ascii
[Wallpaper Sources] --> [Download Manager] --> [Image Processor] --> [Wallpaper Setter]
     |                        |                       |                     |
     v                        v                       v                     v
  Bing/NG/...            HTTP/File Access         Crop/Watermark        OS-specific API
```

Component interactions:

1. Channel providers fetch metadata and images from configured sources
2. Download manager handles HTTP requests with proxy support
3. Image processor applies cropping and watermarks
4. Platform-specific setters update the desktop wallpaper
5. History manager tracks downloaded images to avoid duplicates

### Infrastructure

The project uses GitHub Actions for CI/CD with the following key components:

#### Build Workflow

- Triggers: push, pull_request, workflow_dispatch
- Jobs:
  - build-windows-amd64: Windows builds
  - build-linux: Linux builds (amd64)
  - build-darwin: macOS builds (amd64, arm64)
  - release: Creates GitHub releases for tagged versions
