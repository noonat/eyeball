import type { Section } from "./Sheet";

/**
 * The foundations sheet's sections, in the order docs/design/foundations.html
 * gives them, each with the copy that explains its specimens. A section lists
 * the states of its primitive by story id, and the sheet's own story fails
 * when one names a story that does not exist or a story is left out.
 */
export const sections: readonly Section[] = [
  {
    id: "palette",
    title: "Palette",
    tag: "tokens",
    intro: (
      <>
        Ink on paper, and one highlighter. The accent is the ink itself, so a
        link and a button read as ink, and <code>--hl</code> is the only color
        that asks for attention. Two tones per semantic hue. The plain tone
        reads as text, the <code>-vivid</code> tone fills rules and borders. One
        value cannot do both in light mode, because an amber dark enough to read
        as text is brown. Additions are teal and removals are clay, never red
        against green, which is the one pair some readers cannot separate at
        all.
      </>
    ),
    panels: [{ states: ["foundations-palette--tokens"] }],
    notes: [
      <>
        <b>Teal and clay separate on the blue-yellow axis</b>, which survives
        both common forms of red-green color blindness. Hue is still never the
        only signal: a changed row carries a sign in the gutter and a tinted
        background.
      </>,
      <>
        <b>
          The light background is a paper with a trace of green, not pure white
          and never cream.
        </b>{" "}
        Reviews happen outdoors on a phone at full brightness, where white
        panels on a white page lose their edges. Both themes take one neutral
        family at one temperature.
      </>,
      <>
        <b>Text on the highlighter is a fixed ink.</b> <code>--on-hl</code> does
        not change with the theme, because light text on yellow cannot be read.
        The dark highlighter leans toward lemon, away from the amber of a
        warning.
      </>,
      <>
        <b>Every text tone holds 4.5 to 1 on the grounds it sits on</b>, in both
        themes. The light Approve green is <code>#15805a</code>, because white
        on the earlier <code>#1a9b6c</code> was 3.53.
      </>,
    ],
  },
  {
    id: "type",
    title: "Type",
    tag: "scale",
    intro: (
      <>
        Two families, each in one role. Atkinson Hyperlegible Next and its Mono
        set the chrome, code and addresses: the Braille Institute drew them for
        low-vision legibility, and they keep l, I and 1, and 0 and O, apart.
        Literata sets what an agent wrote and a rendered document. The fonts are
        215 KB, served beside the stylesheet, so a first load shows the system
        faces until they arrive.
      </>
    ),
    panels: [{ states: ["foundations-type--scale"] }],
  },
  {
    id: "icons",
    title: "Icons",
    tag: "lucide",
    intro: (
      <>
        <b>Lucide</b>, ISC licensed, drawn on a 24 unit grid with a 2 unit
        stroke. Each icon is inlined into <code>app.css</code> as a CSS mask, so{" "}
        <code>currentColor</code> paints it and it takes the color of whatever
        text it sits beside. Eighteen icons come to about 5.7KB of stylesheet,
        with no font file, no second request, and no load state. The names in{" "}
        <code>icons.txt</code> are the names on lucide.dev.
      </>
    ),
    panels: [
      {
        states: ["foundations-icons--inventory", "foundations-icons--in-text"],
      },
    ],
    notes: [
      <>
        <b>
          A mask rather than a font, and rather than inline SVG in the markup.
        </b>{" "}
        A font would have to ship a thousand icons or add a subsetting step, and
        its failure mode is the worst available: a ligature icon font renders
        the literal word <code>check</code> mid-sentence wherever the shaper
        does not cooperate, after looking correct everywhere it was tested.
        Inline SVG at the use site is correct and puts eighteen blocks of path
        data into the markup. A mask keeps one table in the stylesheet and a
        readable class at the use site.
      </>,
      <>
        <b>
          Presentation attributes are stripped from each SVG and declared once
          in the <code>.i</code> rule.
        </b>{" "}
        Stroke weight is one number rather than eighteen, so making every icon
        heavier at small sizes is a single edit.
      </>,
      <>
        <b>Stroke icons at 2 units read light at 11px.</b> That is the real cost
        of this family against a filled one, and it is the thing to judge on the
        phone rather than here. The fix, if it is needed, is the stroke width in
        that one rule.
      </>,
      <>
        <b>Color is inherited, never set on the icon.</b> A button and a notice
        each tint their own text, and the glyph follows without a second rule.
        Pills keep their dot instead: two marks on one small pill is one more
        than it can carry.
      </>,
      <>
        <b>
          The diff gutter keeps its <code>+</code> and <code>&minus;</code> as
          text.
        </b>{" "}
        Those characters sit in a monospace column and have to align with the
        code beside them. An icon there is a different width and breaks the
        grid.
      </>,
    ],
  },
  {
    id: "state",
    title: "State",
    tag: "pills",
    intro: (
      <>
        Four states, each with a pill. Work waiting on the reviewer is the one
        card on the screen and its title is marked with the highlighter, which
        reads at arm&rsquo;s length as the stripe it replaced did. A pill also
        carries a plain count, such as a round number.
      </>
    ),
    panels: [{ states: ["foundations-pill--tones"] }],
  },
  {
    id: "attention",
    title: "Attention",
    tag: "the eye",
    intro: (
      <>
        The eye is in the top bar of every screen: open with the waiting count
        as its iris while anything waits, shut when nothing does, and large and
        shut as the empty queue.
      </>
    ),
    panels: [
      {
        states: [
          "foundations-topbar--queue",
          "foundations-topbar--screen",
          "foundations-topbar--nothing-waiting",
          "foundations-eye--open",
          "foundations-eye--two-digits",
          "foundations-eye--shut",
          "foundations-eye--large",
          "foundations-emptystate--nothing-waiting",
        ],
      },
    ],
    notes: [
      <>
        <b>Two digits fit the iris.</b> A queue of a hundred is not a case this
        surface is drawn for.
      </>,
      <>
        <b>The eye is drawn in app.css rather than listed in icons.txt.</b>{" "}
        Lucide has no shut eye with lashes, and the open one has to hold a
        count.
      </>,
    ],
  },
  {
    id: "comments",
    title: "Comments",
    tag: "level chips",
    intro: (
      <>
        A comment attaches to a line or range, to a passage, to a file, or to
        the review itself. The review level is the one with nowhere else to go,
        and it carries the most important comment anybody writes, so it is never
        behind a menu.
      </>
    ),
    panels: [{ states: ["foundations-levelchips--levels"] }],
  },
  {
    id: "actions",
    title: "Actions",
    tag: "buttons, notices",
    intro: (
      <>
        Refusing to request changes with nothing to act on is a notice, not a
        disabled button with no explanation.
      </>
    ),
    panels: [
      {
        states: [
          "foundations-button--verdicts",
          "foundations-button--acting",
          "foundations-notice--tones",
        ],
      },
    ],
    notes: [
      <>
        <b>Approve is the filled button and sits on the right</b>, under the
        thumb. Request changes is outlined rather than filled: it is the common
        verdict, and two filled buttons of equal weight make the reviewer read
        both before acting.
      </>,
      <>
        <b>A notice is for what could not have been prevented.</b> It is never a
        refusal of something the screen offered and then declined, which is why
        the warn notice appears beside the bar rather than replacing it.
      </>,
    ],
  },
  {
    id: "firstrun",
    title: "First run",
    tag: "install, permission, badge",
    intro: (
      <>
        Notifications do not arrive until the surface is installed to the home
        screen and granted permission. That makes it the one setup step that has
        to be impossible to miss rather than something found later in a settings
        screen. The badge on the installed icon is what answers &ldquo;is
        anything waiting&rdquo; without opening anything.
      </>
    ),
    panels: [
      {
        states: [
          "foundations-badgeicon--waiting",
          "foundations-steps--first-run",
          "foundations-notice--lapsed",
        ],
      },
    ],
    notes: [
      <>
        <b>
          A lapsed subscription is a state the surface reports, not one it
          hides.
        </b>{" "}
        A queue that looks empty because nothing is arriving is the failure this
        exists to prevent, and it is silent by nature.
      </>,
    ],
  },
];
