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

# One related clue word for each board word that no theme covers, built from
# the official word list (examples/wordlist.txt).
ASSOCIATIONS = dict(pair.split(":") for pair in """
agent:secret air:breathe alien:extraterrestrial amazon:rainforest aztec:empire
band:musicians bar:pub bark:woof beach:sand beat:rhythm bed:sleep
bermuda:caribbean block:lego board:plank bolt:lightning bond:james
boom:explosion box:cardboard bridge:river buck:deer bugle:trumpet
button:shirt capital:uppercase cast:actors cell:prison center:middle
chair:seat chest:treasure circle:circumference compound:chemical
conductor:orchestra contract:signature cotton:fabric cover:blanket
crash:collision cross:crucifix cycle:bicycle dance:ballet date:romance
day:sunlight death:grave degree:diploma dinosaur:fossil draft:breeze
drop:plummet egypt:nile face:smile fair:carnival fan:supporter
fence:barrier fighter:boxer figure:silhouette fire:flame game:puzzle
gas:petrol genius:brilliant grace:elegance green:emerald ground:soil
head:skull hole:dig horseshoe:magnet ivory:elephant kid:child
lawyer:attorney life:alive light:lamp line:queue litter:trash
loch_ness:scotland lock:combination log:timber luck:fortune mail:envelope
mammoth:woolly marble:granite march:parade mass:weight match:ignite
mine:quarry model:catwalk net:mesh night:darkness ninja:stealth
novel:fiction oil:petroleum parachute:skydive park:playground part:piece
pass:overtake paste:glue pilot:cockpit pipe:plumbing pit:hollow play:drama
plot:storyline point:arrow pole:totem post:parcel press:newspaper
ray:beam revolution:uprising ring:wedding robot:android rock:boulder
roulette:wheel round:sphere satellite:orbit saturn:titan scale:balance
shadow:silhouette shakespeare:sonnet shop:store shot:injection
skyscraper:tall slip:banana snowman:frosty soldier:army soul:spirit
sound:noise spell:wizard spike:thorn spine:vertebra spot:dot
square:rectangle staff:employees state:province stick:twig straw:drinking
suit:tuxedo swing:hammock table:furniture tag:label tail:wag tick:clock
time:hour torch:flashlight triangle:geometry trip:journey trunk:luggage
tube:subway turkey:thanksgiving undertaker:funeral vacuum:dust water:drink
wake:alarm wall:brick war:battle washer:laundry watch:wrist whip:lash
yard:lawn force:strength plastic:polymer row:paddle
""".split())

# Every clue we might give, as (clue words to try, board words it points to):
# the themes plus one single-word entry per association.
CANDIDATES = THEMES + [([clue], {word}) for word, clue in ASSOCIATIONS.items()]


def theme_for_clue(clue):
    """Returns the board words a clue word points to, or an empty set."""
    clue = clue.lower()
    out = set()
    for clues, words in CANDIDATES:
        if clue in clues:
            out |= words
    return out


def theme_count(word):
    """How many themes a word appears in (fewer = more specific)."""
    return sum(1 for _, words in THEMES if word in words)
