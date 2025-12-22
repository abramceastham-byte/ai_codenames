from sentence_transformers import SentenceTransformer

# Load the model
model = SentenceTransformer("Qwen/Qwen3-Embedding-0.6B")

# We recommend enabling flash_attention_2 for better acceleration and memory saving,
# together with setting `padding_side` to "left":
# model = SentenceTransformer(
#     "Qwen/Qwen3-Embedding-0.6B",
#     model_kwargs={"attn_implementation": "flash_attention_2", "device_map": "auto"},
#     tokenizer_kwargs={"padding_side": "left"},
# )

# The queries and documents to embed
queries = [
    "satellite",
    "boot",
    "king",
    "mammoth",
    "revolution",
    "knight",
    "hospital",
    "contract",
    "tokyo",
    "novel",
    "vacuum",
    "shot",
    "bond",
    "concert",
    "scuba diver",
    "glass",
    "mine",
    "shop",
    "hole",
    "degree",
    "missile",
    "pie",
    "pool",
    "atlantis",
    "platypus",
]
documents = [
    "war",
]

# Encode the queries and documents. Note that queries benefit from using a prompt
# Here we use the prompt called "query" stored under `model.prompts`, but you can
# also pass your own prompt via the `prompt` argument
query_embeddings = model.encode(queries, prompt="")
document_embeddings = model.encode(documents)

# Compute the (cosine) similarity between the query and document embeddings
similarity = model.similarity(query_embeddings, document_embeddings)

# Zip queries with their similarity scores and sort by score (descending)
results = [(query, score[0].item()) for query, score in zip(queries, similarity)]
results.sort(key=lambda x: x[1], reverse=True)

for query, score in results:
    print(f"{query}: {score:.4f}")
