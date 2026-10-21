"""Word tags shared by our spymaster and operative.

Each tag is a clue word (with backups, in case it's illegal on a board)
and the board words it describes. Tags overlap on purpose, so there are
more chances to cover two or more of our words with one clue.
"""
import os

TAGS = [
    (["animal", "beast", "creature"], "bear bat buffalo calf cat chick crane dinosaur dog duck eagle fish fly hawk horse kangaroo kiwi lion mammoth mole mouse octopus penguin platypus rabbit robin scorpion seal shark slug spider turkey whale worm"),
    (["wing", "flight", "flying"], "angel bat bug crane duck eagle fly hawk helicopter jet parachute phoenix plane pilot robin dragon"),
    (["sea", "ocean", "marine"], "beach bermuda fish octopus pirate port scuba_diver seal shark ship sub wave whale atlantis"),
    (["water", "wet", "liquid"], "beach fish loch_ness pool sink stream tap wave well whale ice washer"),
    (["food", "dinner", "eat"], "apple berry carrot chocolate ham honey ice_cream jam ketchup kiwi lemon mint nut olive orange pie pumpkin turkey"),
    (["sweet", "dessert", "sugar"], "chocolate honey ice_cream jam pie berry apple"),
    (["kitchen", "cooking", "chef"], "cook fork knife pan plate mug glass bottle oil table paste"),
    (["europe", "european"], "alps berlin czech england france germany greece london loch_ness moscow rome olympus"),
    (["asia", "asian"], "beijing china india tokyo himalayas ninja"),
    (["america", "usa", "american"], "amazon aztec canada hollywood mexico new_york washington"),
    (["nation", "country", "flag"], "america australia canada china czech england france germany greece india mexico turkey state"),
    (["city", "urban", "downtown"], "beijing berlin london moscow rome tokyo new_york washington skyscraper"),
    (["ancient", "history", "antique"], "aztec pyramid temple rome greece olympus egypt mammoth dinosaur knight"),
    (["body", "anatomy"], "arm back eye face foot hand head heart mouth spine thumb tooth tail lap pupil"),
    (["clothes", "wear", "fashion"], "belt boot cap cloak dress glove hood pants shoe sock suit tie model"),
    (["music", "song", "instrument"], "band beat bell bugle concert conductor flute horn note opera organ piano pitch string drum"),
    (["sport", "athlete", "team"], "ball club court cricket fan field goal match net pitch pool racket round stadium strike swing track"),
    (["game", "toy", "fun"], "ball card deck dice game play roulette tag swing kid"),
    (["gamble", "bet", "vegas"], "casino card deck dice roulette jack luck"),
    (["weather", "storm", "forecast"], "cold ice snow wind spring fall draft ray day"),
    (["winter", "frozen", "chilly"], "cold ice snow snowman glove"),
    (["sky", "heaven", "cosmos"], "air angel jupiter moon saturn star mercury satellite space ray comet"),
    (["planet", "orbit", "astronomy"], "jupiter mercury moon saturn satellite space star telescope"),
    (["magic", "fantasy", "fairy"], "centaur dragon dwarf genius ghost giant leprechaun phoenix spell unicorn witch wizard"),
    (["royal", "palace", "throne"], "king queen princess crown knight court castle"),
    (["hospital", "medical", "health"], "ambulance doctor nurse disease hospital vet poison spine heart"),
    (["crime", "criminal", "illegal"], "thief smuggler spy pistol bomb poison pirate police lawyer court"),
    (["weapon", "armed", "arsenal"], "bomb bow knife laser missile pistol shot spike torch whip"),
    (["army", "military", "battle"], "soldier war missile bomb pistol tank knight fighter shot"),
    (["vehicle", "transport", "travel"], "ambulance car helicopter jet limousine plane ship sub train van trip cycle"),
    (["road", "traffic", "drive"], "car crash limousine van bridge pass track"),
    (["tool", "hardware", "workshop"], "drill hook nail pin needle bolt brush ruler saw lock key"),
    (["mountain", "climb", "peak"], "alps cliff himalayas mount olympus pole"),
    (["money", "wealth", "cash"], "bank bill casino change check gold millionaire pound stock mint"),
    (["tree", "wood", "lumber"], "bark branch log maple palm root forest stick"),
    (["garden", "flower", "plant"], "grass rose root straw worm slug spring palm"),
    (["metal", "mineral", "element"], "copper gold iron lead mercury"),
    (["school", "class", "student"], "teacher school pupil ruler lab paper degree tablet board"),
    (["computer", "internet", "digital"], "server screen web mouse file code key link tablet mail"),
    (["building", "architecture", "construction"], "bridge church embassy hospital hotel school skyscraper stadium temple theater tower wall"),
    (["movie", "cinema", "show"], "film hollywood comic superhero theater screen star cast play"),
    (["science", "experiment", "research"], "lab microscope telescope scientist laser genius compound"),
    (["electric", "power", "energy"], "battery charge engine switch light torch"),
    (["myth", "legend", "folklore"], "atlantis centaur dragon leprechaun loch_ness olympus phoenix unicorn"),
    (["round", "circle", "ring"], "ball circle cycle ring round moon wheel plate"),
    (["shape", "geometry"], "circle square triangle point line figure"),
    (["time", "clock", "calendar"], "date day march night watch tick time spring fall"),
    (["paper", "write", "letter"], "mail note novel paper post press draft contract file"),
    (["fire", "flame", "heat"], "fire torch light gas oil match"),
    (["death", "grave", "dark"], "death undertaker ghost shadow soul poison night"),
    (["job", "career", "profession"], "doctor lawyer nurse pilot scientist soldier teacher undertaker cook model conductor"),
    (["hero", "villain", "comic"], "superhero ninja pirate knight genius"),
    (["continent", "world", "globe"], "africa antarctica europe america asia australia"),
    (["spy", "secret", "undercover"], "agent bond spy ninja smuggler code"),
    (["space", "alien", "ufo"], "alien robot satellite saturn jupiter moon star"),
    (["machine", "robotic", "gadget"], "robot vacuum engine washer laser battery switch"),
    (["furniture", "home", "house"], "bed chair table chest cover lamp sink"),
    (["container", "storage", "packing"], "box chest trunk bottle mug pipe tube"),
    (["explosion", "blast", "bang"], "boom bomb crash missile shot"),
    (["jewel", "gem", "jewelry"], "diamond ring crown gold ivory marble"),
    (["stone", "quarry", "mining"], "marble rock mine pit diamond gold iron"),
    (["deer", "antler", "hunting"], "buck bow"),
    (["shirt", "sew", "fabric"], "button cotton cloak dress tie"),
    (["prison", "jail", "cage"], "cell bar block police thief"),
    (["middle", "core", "inside"], "center heart cell"),
    (["government", "politics", "capitol"], "capital state embassy washington revolution"),
    (["dance", "ballet", "disco"], "dance concert opera"),
    (["fall", "gravity", "tumble"], "drop slip fall"),
    (["carnival", "festival", "fun"], "fair circus"),
    (["barrier", "border", "boundary"], "fence wall line"),
    (["physics", "newton", "gravity"], "force mass gravity"),
    (["elegance", "beauty", "charm"], "grace rose model"),
    (["color", "colour", "paint"], "green orange gold copper ivory"),
    (["earth", "soil", "dirt"], "ground root worm hole pit"),
    (["lucky", "fortune", "luck"], "horseshoe luck clover"),
    (["life", "alive", "living"], "life soul heart"),
    (["trash", "garbage", "rubbish"], "litter"),
    (["park", "playground", "outdoor"], "park swing yard grass"),
    (["piece", "portion", "section"], "part pass"),
    (["plumbing", "plumber", "drain"], "pipe sink tap tube water"),
    (["plastic", "polymer", "synthetic"], "plastic"),
    (["story", "storyline", "narrative"], "plot novel comic"),
    (["shakespeare", "playwright", "drama"], "shakespeare opera theater play"),
    (["boat", "oar", "paddle"], "row ship sub"),
    (["weight", "measure", "balance"], "scale pound ruler"),
    (["store", "retail", "shopping"], "shop mall"),
    (["noise", "hear", "loud"], "sound boom bell horn"),
    (["dot", "spotty", "speckle"], "spot point"),
    (["employee", "workers", "crew"], "staff band"),
    (["sleep", "awake", "dream"], "wake bed night"),
    (["drink", "thirst", "hydrate"], "water bottle mug"),
    (["religion", "holy", "faith"], "church cross temple angel soul"),
]



def load_wordlist():
    """Board words, from a copy of examples/wordlist.txt kept next to us."""
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "wordlist.txt")
    with open(path) as f:
        return set(f.read().split())


# Drop any tag words that can't appear on a board.
WORDLIST = load_wordlist()
TAGS = [(clues, [w for w in words.split() if w in WORDLIST]) for clues, words in TAGS]


def tags_for_clue(clue):
    """All board words described by a clue word."""
    clue = clue.lower()
    out = []
    for clues, words in TAGS:
        if clue in clues:
            out += [w for w in words if w not in out]
    return out


def guess_order(clue, unrevealed):
    """The order our operative tries words for a clue: most certain first,
    then alphabetical so the spymaster can predict it exactly."""
    matches = [w for w in tags_for_clue(clue) if w in unrevealed]
    return sorted(matches, key=lambda w: (tag_count(w), w))


def tag_count(word):
    """How many tags a word has. Fewer means a more certain match."""
    return sum(1 for _, words in TAGS if word in words)
