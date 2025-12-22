from gensim.models import KeyedVectors

# Paths to your downloaded files
NUMBERBATCH_PATH = "../data/numberbatch-en-19.08.txt"
GLOVE_PATH = (
    # "glove.6B.300d.word2vec.txt"  # Assumes you converted GloVe to word2vec format
    "../data/wiki_giga_2024_100_MFT20_vectors_seed_2024_alpha_0.75_eta_0.05.050_combined.txt"
)

print("Loading ConceptNet Numberbatch (Spymaster)...")
# limit=500000 saves RAM by only loading top common words
spymaster_model = KeyedVectors.load_word2vec_format(
    NUMBERBATCH_PATH, binary=False, limit=500000
)

print("Loading GloVe (Operative)...")
operative_model = KeyedVectors.load_word2vec_format(GLOVE_PATH, binary=False)


def get_best_clue(targets, bad_words, assassin, model):
    """
    Generates a clue connecting 'targets' using ConceptNet logic.
    """

    # 1. Generate Candidates: Look at neighbors of the target words
    # We intersection the neighbors to find words that link them together.
    candidate_pool = set()
    for word in targets:
        # Get top 50 related concepts for this word
        neighbors = model.most_similar(word, topn=50)
        for neighbor, _ in neighbors:
            # Clean neighbor key (remove /c/en/ prefix for checking)
            # clean_neighbor = neighbor.replace("_", " ")
            candidate_pool.add(neighbor)

    best_clue = None
    best_score = -float("inf")

    # 2. Score Candidates
    for clue in candidate_pool:
        # Calculate Scores
        # A. Similarity to Targets (We want the clue to fit ALL targets well)
        target_sims = []
        for t in targets:
            sim = model.similarity(clue, t)
            target_sims.append(sim)

        min_target_sim = min(target_sims)  # The "weakest link" determines validitity

        # B. Similarity to Assassin (Death card - huge penalty)
        assassin_sim = model.similarity(clue, assassin)

        # C. Similarity to Bad Words (Opponent/Neutral - medium penalty)
        bad_sims = []
        for b in bad_words:
            bad_sims.append(model.similarity(clue, b))
        max_bad_sim = max(bad_sims) if bad_sims else 0

        # FINAL SCORE FORMULA
        # High lowest-connection to targets - penalty for assassin - penalty for bad words
        score = (min_target_sim * 1.5) - (assassin_sim * 5.0) - (max_bad_sim * 2.0)

        if score > best_score:
            best_score = score
            best_clue = clue

    return best_clue, best_score


def guess_words(clue, count, board_words, model):
    """
    Guesses 'count' words from 'board_words' based on similarity to 'clue'
    using GloVe vectors.
    """
    if clue not in model:
        return [("Clue unknown", 0.0)]

    scores = []

    for word in board_words:
        # Handle multi-word strings on board if necessary (e.g. "ice cream")
        # For simplicity, we assume single words or use average vectors
        if word in model:
            sim = model.similarity(clue, word)
        else:
            sim = 0.0  # Word not in vocab (unlikely with GloVe 6B)

        scores.append((word, sim))

    # Sort by highest similarity
    scores.sort(key=lambda x: x[1], reverse=True)

    return scores[:count]


# Example Usage
# Let's assume the Spymaster said "MEDICINE 3"
current_board = [
    "doctor",
    "hospital",
    "nurse",
    "teacher",
    "spoon",
    "death",
    "apple",
    "moon",
]
clue_given = "medicine"
number_given = 3

guesses = guess_words(clue_given, number_given, current_board, operative_model)

print("\nOperative Guesses:")
for word, conf in guesses:
    print(f"- {word} (Confidence: {conf:.2f})")

# Example Usage
board_targets = ["doctor", "hospital", "nurse"]
board_bad = ["teacher", "spoon"]
board_assassin = "death"

clue, score = get_best_clue(board_targets, board_bad, board_assassin, spymaster_model)
print(f"Spymaster Clue: {clue.upper()} (Count: {len(board_targets)})")
