FROM mcr.microsoft.com/playwright:v1.57.0-jammy

USER root

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      git \
      golang-go \
      python3 \
      python3-pip \
    && python3 -m pip install --no-cache-dir pytest pytest-cov \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -u 1000 -m qsm || true

WORKDIR /workspace
USER 1000:1000
