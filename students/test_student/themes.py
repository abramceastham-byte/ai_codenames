"""Hand-made theme dictionary shared by our spymaster and operative.

Each theme has a list of clue words to try (in order, in case one is illegal
on a given board) and the board words that belong to it.
"""

THEMES = [
    (["animal", "creature"], "bear bat cat dog duck eagle fish horse kangaroo lion mouse octopus penguin platypus rabbit seal shark spider whale worm buffalo hawk crane robin chick calf scorpion slug bug mole fly kiwi"),
    (["bird", "feather"], "eagle hawk crane robin duck penguin chick kiwi phoenix"),
    (["ocean", "marine"], "whale shark seal octopus fish ship sub scuba_diver wave port pirate"),
    (["food", "meal"], "apple berry carrot chocolate honey ice_cream ketchup lemon olive orange pie pumpkin jam ham nut mint"),
    (["fruit", "juicy"], "apple berry lemon olive orange kiwi pumpkin"),
    (["country", "nation"], "africa america australia canada china england france germany greece india mexico czech europe antarctica"),
    (["city", "metropolis"], "beijing berlin london moscow rome tokyo new_york washington hollywood"),
    (["body", "anatomy"], "arm back eye foot hand heart mouth thumb tooth lap"),
    (["clothing", "outfit"], "belt boot cap cloak dress glove hood pants shoe sock tie"),
    (["music", "melody"], "bell concert flute horn note opera organ piano pitch string"),
    (["sport", "athlete"], "ball club court cricket field pitch pool racket stadium track strike"),
    (["weather", "climate"], "snow wind ice cold spring fall"),
    (["astronomy", "planet"], "moon star jupiter mercury telescope space"),
    (["fantasy", "fairytale"], "dragon unicorn centaur leprechaun phoenix giant dwarf witch ghost angel"),
    (["royal", "monarchy"], "king queen princess crown knight"),
    (["medicine", "medical"], "doctor nurse hospital ambulance disease poison vet"),
    (["crime", "criminal"], "thief smuggler spy police pistol bomb poison pirate"),
    (["weapon", "armed"], "pistol bomb missile laser knife bow"),
    (["vehicle", "transport"], "car plane jet train helicopter limousine van ambulance"),
    (["kitchen", "cooking"], "knife fork pan plate mug glass bottle cook"),
    (["tool", "toolbox"], "drill hook nail pin needle brush ruler"),
    (["mountain", "summit"], "alps himalayas olympus cliff mount"),
    (["money", "cash"], "bank bill casino check millionaire pound stock gold change"),
    (["poker", "gambling"], "card deck dice casino jack club diamond"),
    (["plant", "botany"], "palm maple root olive forest grass rose"),
    (["metal", "mineral"], "gold copper iron lead mercury"),
    (["classroom", "student"], "teacher school pupil lab ruler paper tablet"),
    (["computer", "internet"], "server screen web mouse file code key link"),
    (["building", "architecture"], "hotel hospital church temple tower embassy stadium theater"),
    (["movie", "cinema"], "hollywood film comic superhero theater screen"),
    (["science", "experiment"], "lab microscope telescope scientist laser"),
    (["liquid", "wet"], "pool stream wave sink tap well"),
    (["electric", "electricity"], "battery switch charge engine"),
    (["myth", "legend"], "centaur unicorn phoenix dragon olympus atlantis giant"),
    (["egypt", "pharaoh"], "pyramid temple"),
]

THEMES = [(clues, set(words.split())) for clues, words in THEMES]


def theme_for_clue(clue):
    """Returns the board words for a clue word, or an empty set."""
    clue = clue.lower()
    for clues, words in THEMES:
        if clue in clues:
            return words
    return set()


def theme_count(word):
    """How many themes a word appears in (fewer = more specific)."""
    return sum(1 for _, words in THEMES if word in words)
