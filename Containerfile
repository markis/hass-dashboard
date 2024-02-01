FROM python:3.11 as builder
ARG BUILD_VERSION=0.10.0

WORKDIR /src
ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
  PIP_DISABLE_ROOT_WARNING=1 \
  PIP_ROOT_USER_ACTION=ignore \
  PIP_CACHE_DIR="/var/cache/pip/" \
  HATCH_BUILD_HOOK_ENABLE_MYPYC=1

COPY pyproject.toml /src/
COPY dashboard/ /src/dashboard/
RUN --mount=type=cache,target=/var/cache/pip/ \
  --mount=type=bind,src=.git,dst=/src/.git \
  pip install build~="$BUILD_VERSION"; \
  python -m build --wheel


FROM python:3.11-slim as runtime
ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
  PIP_DISABLE_ROOT_WARNING=1 \
  PIP_ROOT_USER_ACTION=ignore \
  PIP_CACHE_DIR="/var/cache/pip/" \
  HATCH_BUILD_HOOK_ENABLE_MYPYC=1 \
  TINI_VERSION="v0.19.0"

ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-arm64 /tini
RUN chmod +x /tini
ENTRYPOINT ["/tini", "--"]
CMD ["generate_dashboard"]
WORKDIR "/src"

RUN groupadd --system --gid 888 bot && \
  useradd --system --uid 888 --no-user-group --gid 888 \
  --create-home --home-dir /home/bot --shell /bin/bash bot

RUN --mount=type=cache,target=/var/lib/apt/lists \
  --mount=type=cache,target=/var/cache \
  --mount=type=tmpfs,target=/var/log \
  apt-get -y update; apt-get install -y chromium fonts-font-awesome fonts-lato fontconfig; \
  fc-cache -f -v

RUN --mount=type=cache,target=/var/cache/pip/ \
  --mount=type=bind,from=builder,src=/src/dist,target=/src/dist \
  pip install /src/dist/*.whl;

USER bot
