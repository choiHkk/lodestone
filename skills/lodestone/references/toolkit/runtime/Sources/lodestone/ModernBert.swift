import Foundation
import MLX
import MLXNN

struct ModernBertConfiguration: Decodable, Sendable {
    struct Quantization: Decodable, Sendable {
        let groupSize: Int
        let bits: Int
        let mode: String?

        enum CodingKeys: String, CodingKey {
            case groupSize = "group_size"
            case bits
            case mode
        }
    }

    let modelType: String
    let vocabularySize: Int
    let hiddenSize: Int
    let intermediateSize: Int
    let hiddenLayers: Int
    let attentionHeads: Int
    let normEps: Float
    let localAttention: Int
    let globalAttentionEvery: Int

    func isGlobalLayer(_ index: Int) -> Bool {
        index % globalAttentionEvery == 0
    }
    let globalRopeTheta: Float
    let localRopeTheta: Float
    let padTokenID: Int
    let quantization: Quantization?

    enum CodingKeys: String, CodingKey {
        case modelType = "model_type"
        case vocabularySize = "vocab_size"
        case hiddenSize = "hidden_size"
        case intermediateSize = "intermediate_size"
        case hiddenLayers = "num_hidden_layers"
        case attentionHeads = "num_attention_heads"
        case normEps = "norm_eps"
        case localAttention = "local_attention"
        case globalAttentionEvery = "global_attn_every_n_layers"
        case globalRopeTheta = "global_rope_theta"
        case localRopeTheta = "local_rope_theta"
        case padTokenID = "pad_token_id"
        case quantization
    }
}

final class ModernBertEmbeddings: Module {
    @ModuleInfo(key: "tok_embeddings") var tokenEmbeddings: Embedding
    @ModuleInfo(key: "norm") var norm: LayerNorm

    init(_ config: ModernBertConfiguration) {
        _tokenEmbeddings.wrappedValue = Embedding(
            embeddingCount: config.vocabularySize,
            dimensions: config.hiddenSize
        )
        _norm.wrappedValue = LayerNorm(
            dimensions: config.hiddenSize,
            eps: config.normEps,
            bias: false
        )
    }

    func callAsFunction(_ inputIDs: MLXArray) -> MLXArray {
        norm(tokenEmbeddings(inputIDs))
    }
}

final class ModernBertMLP: Module {
    @ModuleInfo(key: "Wi") var inputProjection: Linear
    @ModuleInfo(key: "Wo") var outputProjection: Linear

    init(_ config: ModernBertConfiguration) {
        _inputProjection.wrappedValue = Linear(
            config.hiddenSize,
            config.intermediateSize * 2,
            bias: false
        )
        _outputProjection.wrappedValue = Linear(
            config.intermediateSize,
            config.hiddenSize,
            bias: false
        )
    }

    func callAsFunction(_ inputs: MLXArray) -> MLXArray {
        let parts = inputProjection(inputs).split(parts: 2, axis: -1)
        return outputProjection(silu(parts[0]) * parts[1])
    }
}

final class ModernBertAttention: Module {
    @ModuleInfo(key: "Wqkv") var inputProjection: Linear
    @ModuleInfo(key: "Wo") var outputProjection: Linear

    let attentionHeads: Int
    let headDimension: Int
    let scale: Float
    let rope: RoPE

    init(_ config: ModernBertConfiguration, layerIndex: Int) {
        attentionHeads = config.attentionHeads
        headDimension = config.hiddenSize / config.attentionHeads
        scale = pow(Float(headDimension), -0.5)
        _inputProjection.wrappedValue = Linear(
            config.hiddenSize,
            config.hiddenSize * 3,
            bias: false
        )
        _outputProjection.wrappedValue = Linear(
            config.hiddenSize,
            config.hiddenSize,
            bias: false
        )
        let isGlobal = config.isGlobalLayer(layerIndex)
        rope = RoPE(
            dimensions: headDimension,
            traditional: false,
            base: isGlobal ? config.globalRopeTheta : config.localRopeTheta
        )
    }

    func callAsFunction(_ inputs: MLXArray, mask: MLXArray) -> MLXArray {
        let batch = inputs.dim(0)
        let length = inputs.dim(1)
        let projected = inputProjection(inputs)
            .reshaped(batch, length, 3, attentionHeads, headDimension)
            .transposed(0, 3, 2, 1, 4)
        let parts = projected.split(parts: 3, axis: 2)
        let queries = rope(parts[0].squeezed(axis: 2))
        let keys = rope(parts[1].squeezed(axis: 2))
        let values = parts[2].squeezed(axis: 2)
        let attended = MLXFast.scaledDotProductAttention(
            queries: queries,
            keys: keys,
            values: values,
            scale: scale,
            mask: mask
        )
        return outputProjection(
            attended.transposed(0, 2, 1, 3).reshaped(batch, length, -1)
        )
    }
}

final class ModernBertLayer: Module {
    @ModuleInfo(key: "attn_norm") var attentionNorm: LayerNorm?
    @ModuleInfo(key: "attn") var attention: ModernBertAttention
    @ModuleInfo(key: "mlp") var mlp: ModernBertMLP
    @ModuleInfo(key: "mlp_norm") var mlpNorm: LayerNorm

    let isGlobal: Bool

    init(_ config: ModernBertConfiguration, layerIndex: Int) {
        isGlobal = config.isGlobalLayer(layerIndex)
        _attentionNorm.wrappedValue = layerIndex == 0
            ? nil
            : LayerNorm(dimensions: config.hiddenSize, eps: config.normEps, bias: false)
        _attention.wrappedValue = ModernBertAttention(config, layerIndex: layerIndex)
        _mlp.wrappedValue = ModernBertMLP(config)
        _mlpNorm.wrappedValue = LayerNorm(
            dimensions: config.hiddenSize,
            eps: config.normEps,
            bias: false
        )
    }

    func callAsFunction(
        _ inputs: MLXArray,
        globalMask: MLXArray,
        localMask: MLXArray
    ) -> MLXArray {
        let normalized = attentionNorm?(inputs) ?? inputs
        let attentionOutput = attention(
            normalized,
            mask: isGlobal ? globalMask : localMask
        )
        let hidden = inputs + attentionOutput
        return hidden + mlp(mlpNorm(hidden))
    }
}

final class ModernBertBackbone: Module {
    @ModuleInfo(key: "embeddings") var embeddings: ModernBertEmbeddings
    let layers: [ModernBertLayer]
    @ModuleInfo(key: "final_norm") var finalNorm: LayerNorm

    let config: ModernBertConfiguration

    init(_ config: ModernBertConfiguration) {
        self.config = config
        _embeddings.wrappedValue = ModernBertEmbeddings(config)
        layers = (0 ..< config.hiddenLayers).map {
            ModernBertLayer(config, layerIndex: $0)
        }
        _finalNorm.wrappedValue = LayerNorm(
            dimensions: config.hiddenSize,
            eps: config.normEps,
            bias: false
        )
    }

    func callAsFunction(
        _ inputIDs: MLXArray,
        attentionMask: MLXArray
    ) -> MLXArray {
        var hidden = embeddings(inputIDs)
        let batch = inputIDs.dim(0)
        let length = inputIDs.dim(1)
        let keyMask = which(
            attentionMask .== 1,
            Float(0),
            Float(-10_000)
        )
        .asType(hidden.dtype)
        .expandedDimensions(axes: [1, 2])
        let globalMask = broadcast(keyMask, to: [batch, 1, length, length])
        let positions = MLXArray.arange(length)
        let distances = abs(
            positions.expandedDimensions(axis: 0)
                - positions.expandedDimensions(axis: 1)
        )
        let window = (distances .<= (config.localAttention / 2))
            .expandedDimensions(axes: [0, 1])
        let localMask = which(window, globalMask, Float(-10_000)).asType(hidden.dtype)

        for layer in layers {
            hidden = layer(hidden, globalMask: globalMask, localMask: localMask)
        }
        return finalNorm(hidden)
    }
}

final class ModernBertModel: Module {
    @ModuleInfo(key: "model") var model: ModernBertBackbone

    let config: ModernBertConfiguration

    init(_ config: ModernBertConfiguration) {
        self.config = config
        _model.wrappedValue = ModernBertBackbone(config)
    }

    func callAsFunction(
        _ inputIDs: MLXArray,
        attentionMask: MLXArray
    ) -> MLXArray {
        model(inputIDs, attentionMask: attentionMask)
    }
}
