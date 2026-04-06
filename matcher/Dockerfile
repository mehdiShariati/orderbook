FROM rust:1.75-bookworm AS build
WORKDIR /src
COPY Cargo.toml ./
COPY src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN useradd -r -u 10001 matcher
USER matcher
COPY --from=build /src/target/release/matcher /usr/local/bin/matcher
EXPOSE 9090
ENV MATCHER_ADDR=0.0.0.0:9090
ENTRYPOINT ["/usr/local/bin/matcher"]
