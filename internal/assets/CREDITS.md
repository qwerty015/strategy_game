# Credits

The legacy placeholder art in `tiles/` and `units/` is cropped from Kenney's
**"Medieval RTS"** pack — https://kenney.nl/assets/medieval-rts

License: CC0 1.0 (Creative Commons Zero / Public Domain). Free to use in
personal, educational and commercial projects; credit is appreciated but
not required. See https://creativecommons.org/publicdomain/zero/1.0/

The active sprites in `generated/` were generated specifically for this
prototype with the built-in OpenAI image generation tool on 2026-08-25 and
2026-08-26.
The transparent `tree_stages.png` atlas contains three growth stages and is
sliced into individual frames by the game at startup. Other active sprites
were then reduced to 64×64 transparent/opaque game sprites. The cobblestone road
tile, the three-stage tree atlas, the Lumberjack Hut, lumberjack, Winery and
Winemaker sprites were generated in the same pass on 2026-08-25. They are original
project art, not a recreation of any specific existing commercial game's
assets. The source atlases remain outside the repository; the cropped files
in `generated/` are the assets used by the game.

`building_fisher_hut.png`, `unit_fisherman.png`, `unit_fishing_boat.png` and
the three-stage transparent `fish_stages.png` atlas were generated on
2026-08-26. The fish atlas is sliced into three frames by the game at startup.

`building_pig_farm.png`, `building_meat_workshop.png`, `unit_swineherd.png`
and `unit_butcher.png` were generated with the same tool on 2026-08-26 for the
pig and sausage production chain. They are original project art and are not
derived from any specific commercial game's visual assets.

`terrain_stone_deposit.png`, `building_quarry_hut.png`,
`building_carpentry_workshop.png`, `unit_quarryman.png` and
`unit_carpenter.png` were generated with the same tool on 2026-08-26 for
the stone and wood-processing chains. The deposit sprite is a transparent
object drawn over the stone terrain; the buildings and professions use their
own distinct silhouettes and are original project art.

`building_farm_v2.png`, `building_bakery_v2.png` and
`building_winery_v2.png` were generated with the built-in OpenAI image
generation tool on 2026-08-31. They are transparent original project art for
the farm-to-food chain, designed to share the construction set's warm timber,
stone and terracotta palette. Their visual direction uses broad medieval
pixel-art study only; no commercial game pixels were copied or modified.
`building_warehouse_v3.png`, `building_tavern_v2.png` and
`building_lumberjack_hut_v2.png` were generated with the built-in OpenAI
image generation tool on 2026-08-31. They are transparent original project
art for storage, food service and forestry. Their shared dark roof, timber and
stone palette is a new project art direction informed only by broad medieval
pixel-art study; no commercial game pixels were copied or modified.
`building_fisher_hut_v2.png`, `building_pig_farm_v2.png` and
`building_meat_workshop_v2.png` were generated with the built-in OpenAI image
generation tool on 2026-08-31. They are transparent original project art for
fishing and meat production. The fisherman hut has a deliberate south-facing
pier so the renderer can rotate it toward each cardinal shore; the meat
workshop deliberately contains no baked smoke because the renderer supplies
that motion separately. Their visual direction uses broad medieval pixel-art
study only; no commercial game pixels were copied or modified.
`building_carpentry_workshop_v2.png`, `building_quarry_hut_v2.png` and
`building_miner_hut_v2.png` were generated with the built-in OpenAI image
generation tool on 2026-08-31. They are transparent original project art for
lumber processing, stone quarrying and ore mining. The three silhouettes use
visible boards, cut blocks and dark ore respectively so their professions can
be recognized at map scale. Their visual direction uses broad medieval
pixel-art study only; no commercial game pixels were copied or modified.
The current construction set uses `unit_builder.png`,
`construction_foundation_v2.png` and `construction_scaffolding_v2.png`.
The two construction-stage frames were generated with the built-in OpenAI
image generation tool on 2026-08-31 as transparent original project art.
They are generic frames rendered at the footprint of an unfinished road or
building; their medieval pixel-art direction was informed only by broad
visual study, not by copying or modifying a commercial game asset. The
previous construction PNGs remain in the repository as unused fallback art.

`terrain_coal_deposit.png`, `terrain_gold_ore_deposit.png`,
`terrain_iron_ore_deposit.png`, `building_miner_hut.png`,
`building_smeltery.png`, `unit_miner.png` and `unit_smelter.png` were
generated with the built-in OpenAI image generation tool on 2026-08-28 for
the ore-and-smelting chain. They are transparent original project art: each
ore type has its own deposit silhouette, and the two buildings and professions
are visually distinct from the quarry and carpentry chain.
`ambient_hare_run.png` was generated with the built-in OpenAI image generation tool on 2026-08-29. It is a three-frame transparent running-hare atlas, sliced at startup for the ambient visual layer. It is original project art and does not represent or derive from a specific commercial game asset.


building_warehouse_v2.png was generated with the built-in OpenAI image generation tool on 2026-08-29 as a distinct warehouse sprite for this prototype. It is original project art and does not derive from a specific commercial game asset.

`ambient_butterfly_flap.png` was generated with the built-in OpenAI image generation tool on 2026-08-29 as a three-frame transparent butterfly atlas for the cosmetic ambient layer. It is original project art and does not derive from a specific commercial game asset.

`ambient_fox_trot.png` was generated with the built-in OpenAI image generation tool on 2026-08-29 as a three-frame transparent fox atlas for the cosmetic ambient layer. It is original project art and does not derive from a specific commercial game asset.

`ambient_bird_flap.png` was generated with the built-in OpenAI image generation tool on 2026-08-29. It is an original three-frame transparent flying-bird atlas (wings raised, extended and lowered) for the cosmetic ambient layer; it does not derive from a specific commercial game asset.

`unit_*_walk.png` (serf, farmer, baker, winemaker, fisherman, swineherd, butcher, carpenter, lumberjack, quarryman, builder, miner and smelter) plus `unit_fishing_boat_paddle.png` were generated with the built-in OpenAI image generation tool on 2026-08-29. They are original three-frame transparent walking/paddling atlases for the map render layer; the prior static `unit_*.png` files remain the menu and inspector icons.

`buildings/farm/construction_foundation.png`,
`buildings/farm/construction_waiting.png` and
`buildings/farm/construction_finishing.png` were generated with the built-in
OpenAI image generation tool on 2026-08-31 for the per-building construction
pipeline. They respectively depict site preparation, material delivery and a
partly-raised timber frame. They are original transparent project art; their
art direction is informed only by broad medieval RTS visual study, with no
commercial game pixels, source assets or named-building silhouettes copied.
`resources/*/{icon,carry,stack}.png` are original project pixel-art assets
created on 2026-08-31 for the external visual pack. Each current economy
resource has three transparent variants: a compact UI icon, an item carried by
a worker, and a stack for future world/storage rendering. They are simple
programmatic pixel illustrations created specifically for this prototype and
are not copied, derived or traced from any commercial game asset.