.PHONY: all setup build clean test rust-build python-build dsl-build integration-build examples

# Default target
all: build

# Setup development environment
setup:
	@echo "Setting up development environment..."
	cd rust-core && cargo update
	cd python-layer && python -m pip install -e ".[dev]"
	@echo "Setup complete!"

# Build all components
build: rust-build python-build dsl-build integration-build

# Build Rust components
rust-build:
	@echo "Building Rust core..."
	cd rust-core && cargo build --release
	@echo "Rust build complete!"

# Build Python components
python-build:
	@echo "Building Python layer..."
	cd python-layer && python -m pip install -e .
	@echo "Python build complete!"

# Build DSL components
dsl-build:
	@echo "Building DSL..."
	cd dsl && cargo build --release
	@echo "DSL build complete!"

# Build integration components
integration-build:
	@echo "Building integration layer..."
	@echo "Integration build complete!"

# Run tests
test:
	@echo "Running tests..."
	cd rust-core && cargo test
	cd python-layer && python -m pytest
	@echo "Tests complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	cd rust-core && cargo clean
	find . -type d -name "__pycache__" -exec rm -rf {} +
	find . -type d -name "*.egg-info" -exec rm -rf {} +
	find . -type d -name "*.dist-info" -exec rm -rf {} +
	find . -name "*.pyc" -delete
	@echo "Clean complete!"

# Run examples
examples:
	@echo "Running examples..."
	@echo "Examples complete!"
