// swift-tools-version: 6.1

import PackageDescription

let package = Package(
    name: "Lodestone",
    platforms: [.macOS(.v14)],
    dependencies: [
        .package(
            url: "https://github.com/ml-explore/mlx-swift-lm.git",
            revision: "25b00d4e22e61ec9c41efda47990cd2084ec87ff"
        ),
        .package(
            url: "https://github.com/ml-explore/mlx-swift.git",
            exact: "0.31.4"
        ),
    ],
    targets: [
        .executableTarget(
            name: "lodestone",
            dependencies: [
                .product(name: "MLX", package: "mlx-swift"),
                .product(name: "MLXNN", package: "mlx-swift"),
                .product(name: "MLXEmbedders", package: "mlx-swift-lm"),
                .product(name: "MLXLMCommon", package: "mlx-swift-lm"),
            ]
        )
    ]
)
