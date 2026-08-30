import Foundation
import MLX
import MLXEmbedders
import MLXLMCommon
import MLXNN

private struct Request: Decodable {
    let id: Int
    let texts: [String]
}

private struct Response: Encodable {
    let id: Int?
    let embeddings: [[Float]]?
    let dimension: Int?
    let elapsedMilliseconds: Double?
    let error: String?
}

private struct BaseConfiguration: Decodable {
    let modelType: String

    enum CodingKeys: String, CodingKey {
        case modelType = "model_type"
    }
}

@main
private enum EmbeddingRuntime {
    static func main() async {
        do {
            let options = try Options(CommandLine.arguments)
            let modelURL = URL(filePath: options.modelPath, directoryHint: .isDirectory)
            let configData = try Data(contentsOf: modelURL.appending(component: "config.json"))
            let config = try JSONDecoder().decode(BaseConfiguration.self, from: configData)
            switch config.modelType {
            case "qwen3":
                try await runQwen(options: options, modelURL: modelURL)
            case "modernbert":
                try await runModernBert(options: options, modelURL: modelURL)
            default:
                throw RuntimeError.unsupportedModel(config.modelType)
            }
        } catch {
            FileHandle.standardError.write(Data("lodestone runtime: \(error)\n".utf8))
            Foundation.exit(1)
        }
    }

    private static func runQwen(options: Options, modelURL: URL) async throws {
        let configuration = MLXEmbedders.ModelConfiguration(directory: modelURL)
        let container = try await MLXEmbedders.loadModelContainer(configuration: configuration)
        try await serve(options: options) { texts in
            await container.perform { model, tokenizer, pooler in
                let tokenRows = texts.map { text in
                    truncated(tokenizer.encode(text: text, addSpecialTokens: true), to: options.maxTokens)
                }
                let padID = tokenizer.convertTokenToId("<|endoftext|>")
                    ?? tokenizer.eosTokenId
                    ?? 0
                let maxLength = max(1, tokenRows.map(\.count).max() ?? 0)
                let padded = stacked(tokenRows.map { tokens in
                    MLXArray(tokens + Array(repeating: padID, count: maxLength - tokens.count))
                })
                let mask = lengthMask(rows: tokenRows, length: maxLength)
                let output = model(
                    padded,
                    positionIds: nil,
                    tokenTypeIds: nil,
                    attentionMask: mask
                )
                let pooled = pooler(output, mask: mask, normalize: true)
                pooled.eval()

                return pooled.map { $0.asArray(Float.self) }
            }
        }
    }

    private static func runModernBert(options: Options, modelURL: URL) async throws {
        let configData = try Data(contentsOf: modelURL.appending(component: "config.json"))
        let config = try JSONDecoder().decode(ModernBertConfiguration.self, from: configData)
        guard config.globalAttentionEvery > 0, config.attentionHeads > 0,
            config.hiddenSize % config.attentionHeads == 0
        else {
            throw RuntimeError.unsupportedModel("malformed modernbert configuration")
        }
        let model = ModernBertModel(config)
        let weights = try loadArrays(url: modelURL.appending(component: "model.safetensors"))
        if let quantization = config.quantization {
            if let mode = quantization.mode, mode != "affine" {
                throw RuntimeError.unsupportedQuantization(mode)
            }
            quantize(model: model) { path, _ in
                weights["\(path).scales"] == nil
                    ? nil
                    : (
                        groupSize: quantization.groupSize,
                        bits: quantization.bits,
                        mode: QuantizationMode.affine
                    )
            }
        }
        try model.update(parameters: ModuleParameters.unflattened(weights), verify: [.all])
        eval(model)

        let configuration = MLXLMCommon.ModelConfiguration(directory: modelURL)
        let tokenizer = try await loadTokenizer(configuration: configuration, hub: defaultHubApi)
        try await serve(options: options) { texts in
            let tokenRows = texts.map { text in
                truncated(tokenizer.encode(text: text, addSpecialTokens: true), to: options.maxTokens)
            }
            let maxLength = max(1, tokenRows.map(\.count).max() ?? 0)
            let padded = stacked(tokenRows.map { tokens in
                MLXArray(
                    tokens
                        + Array(repeating: config.padTokenID, count: maxLength - tokens.count)
                )
            })
            let mask = lengthMask(rows: tokenRows, length: maxLength)
            let hidden = model(padded, attentionMask: mask)
            let pooled = hidden[0..., 0, 0...].asType(.float32)
            let norms = sqrt(sum(pooled * pooled, axis: -1, keepDims: true))
            let normalized = pooled / norms
            normalized.eval()

            return normalized.map { $0.asArray(Float.self) }
        }
    }

    private static func serve(
        options: Options,
        embed: ([String]) async throws -> [[Float]]
    ) async throws {
        let decoder = JSONDecoder()
        while let line = readLine(strippingNewline: true) {
            var requestID: Int?
            do {
                let request = try decoder.decode(Request.self, from: Data(line.utf8))
                requestID = request.id
                guard !request.texts.isEmpty, request.texts.count <= options.maxBatch else {
                    throw RuntimeError.invalidBatch
                }
                let started = ContinuousClock.now
                let embeddings = try await embed(request.texts)
                let elapsed = started.duration(to: .now)
                write(Response(
                    id: request.id,
                    embeddings: embeddings,
                    dimension: embeddings.first?.count,
                    elapsedMilliseconds: elapsed.milliseconds,
                    error: nil
                ))
            } catch {
                write(Response(
                    id: requestID,
                    embeddings: nil,
                    dimension: nil,
                    elapsedMilliseconds: nil,
                    error: String(describing: error)
                ))
            }
        }
    }

    private static func write(_ response: Response) {
        do {
            var data = try JSONEncoder().encode(response)
            data.append(0x0A)
            try FileHandle.standardOutput.write(contentsOf: data)
        } catch {
            // A response that cannot reach the client would deadlock it;
            // dying is the recoverable outcome.
            FileHandle.standardError.write(Data("write response: \(error)\n".utf8))
            Foundation.exit(1)
        }
    }

    private static func truncated(_ tokens: [Int], to limit: Int) -> [Int] {
        guard limit > 1, tokens.count > limit, let terminal = tokens.last else {
            return tokens
        }

        return Array(tokens.prefix(limit - 1)) + [terminal]
    }

    private static func lengthMask(rows: [[Int]], length: Int) -> MLXArray {
        stacked(rows.map { tokens in
            MLXArray((0..<length).map { $0 < tokens.count })
        })
    }
}

private struct Options {
    let modelPath: String
    let maxBatch: Int
    let maxTokens: Int

    init(_ arguments: [String]) throws {
        let values = Array(arguments.dropFirst())
        let known: Set<String> = ["--model", "--max-batch", "--max-tokens"]
        var index = 0
        while index < values.count {
            guard known.contains(values[index]), index + 1 < values.count else {
                throw RuntimeError.invalidOptions
            }
            index += 2
        }
        guard let modelIndex = values.firstIndex(of: "--model"), modelIndex + 1 < values.count else {
            throw RuntimeError.missingModel
        }
        modelPath = values[modelIndex + 1]
        maxBatch = try values.integer(after: "--max-batch", default: 16)
        maxTokens = try values.integer(after: "--max-tokens", default: 768)
        guard maxBatch > 0, maxTokens > 1 else {
            throw RuntimeError.invalidOptions
        }
    }
}

private extension Array where Element == String {
    func integer(after name: String, default defaultValue: Int) throws -> Int {
        guard let index = firstIndex(of: name) else {
            return defaultValue
        }
        guard index + 1 < count, let value = Int(self[index + 1]) else {
            throw RuntimeError.invalidOptions
        }

        return value
    }
}

private extension Duration {
    var milliseconds: Double {
        let parts = components
        return Double(parts.seconds) * 1_000 + Double(parts.attoseconds) / 1e15
    }
}

private enum RuntimeError: Error {
    case invalidBatch
    case invalidOptions
    case missingModel
    case unsupportedModel(String)
    case unsupportedQuantization(String)
}
