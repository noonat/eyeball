import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { FilesScreen } from "../components/FilesScreen";
import {
  fileWhole,
  filePlain,
  fileDocument,
  fileDocumentSource,
  fileContext,
  fileLive,
  fileImage,
  fileMode,
  fileWorktree,
  filePage,
  fileImageChanged,
  fileImageNew,
  fileImageDeleted,
  fileSvg,
  fileAudioChanged,
  fileVideoNew,
  filePdf,
  fileTooLarge,
  fileBinary,
} from "../fixtures";
import fileWholeHtml from "../fixtures/file-whole.html?raw";
import filePlainHtml from "../fixtures/file-plain.html?raw";
import fileDocumentHtml from "../fixtures/file-document.html?raw";
import fileDocumentSourceHtml from "../fixtures/file-document-source.html?raw";
import fileContextHtml from "../fixtures/file-context.html?raw";
import fileLiveHtml from "../fixtures/file-live.html?raw";
import fileImageHtml from "../fixtures/file-image.html?raw";
import fileModeHtml from "../fixtures/file-mode.html?raw";
import fileWorktreeHtml from "../fixtures/file-worktree.html?raw";
import filePageHtml from "../fixtures/file-page.html?raw";
import fileImageChangedHtml from "../fixtures/file-image-changed.html?raw";
import fileImageNewHtml from "../fixtures/file-image-new.html?raw";
import fileImageDeletedHtml from "../fixtures/file-image-deleted.html?raw";
import fileSvgHtml from "../fixtures/file-svg.html?raw";
import fileAudioChangedHtml from "../fixtures/file-audio-changed.html?raw";
import fileVideoNewHtml from "../fixtures/file-video-new.html?raw";
import filePdfHtml from "../fixtures/file-pdf.html?raw";
import fileTooLargeHtml from "../fixtures/file-too-large.html?raw";
import fileBinaryHtml from "../fixtures/file-binary.html?raw";
import { sameAsApproved } from "./approved";

const meta = {
  title: "File/Screen",
  component: FilesScreen,
  parameters: { frame: "screen" },
} satisfies Meta<typeof FilesScreen>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A code file with the change marked in place. */
export const Whole: Story = {
  args: { files: fileWhole },
  play: async (context) => {
    await sameAsApproved(fileWholeHtml)(context);
    // The table scrolls sideways, so a keyboard has to be able to reach it.
    await expect(context.canvasElement.querySelector(".code")).toHaveAttribute(
      "tabindex",
      "0",
    );
  },
};

/** The same file with the tint switched off. */
export const Plain: Story = {
  args: { files: filePlain },
  play: sameAsApproved(filePlainHtml),
};

/** A markdown file as a document, changes ruled and struck. */
export const Document: Story = {
  args: { files: fileDocument },
  play: sameAsApproved(fileDocumentHtml),
};

/** The same file as source, with the change marked. */
export const DocumentSource: Story = {
  args: { files: fileDocumentSource },
  play: sameAsApproved(fileDocumentSourceHtml),
};

/** A file the review does not touch, read from the base. */
export const Context: Story = {
  args: { files: fileContext },
  play: sameAsApproved(fileContextHtml),
};

/** A file nothing captured, read from disk. */
export const Live: Story = {
  args: { files: fileLive },
  play: sameAsApproved(fileLiveHtml),
};

/** An unchanged image, shown as an image. */
export const ImageContext: Story = {
  args: { files: fileImage },
  play: sameAsApproved(fileImageHtml),
};

/** A file whose mode changed. */
export const Mode: Story = {
  args: { files: fileMode },
  play: sameAsApproved(fileModeHtml),
};

/** A worktree's file, live against HEAD. */
export const Worktree: Story = {
  args: { files: fileWorktree },
  play: sameAsApproved(fileWorktreeHtml),
};

/** A page in its sandboxed frame, with its source one tap away. */
export const Page: Story = {
  args: { files: filePage },
  play: sameAsApproved(filePageHtml),
};

/** A changed image, before over after. */
export const ImageChanged: Story = {
  args: { files: fileImageChanged },
  play: sameAsApproved(fileImageChangedHtml),
};

/** A new image on its own. */
export const ImageNew: Story = {
  args: { files: fileImageNew },
  play: sameAsApproved(fileImageNewHtml),
};

/** A deleted image. */
export const ImageDeleted: Story = {
  args: { files: fileImageDeleted },
  play: sameAsApproved(fileImageDeletedHtml),
};

/** An SVG in its Image view. */
export const Svg: Story = {
  args: { files: fileSvg },
  play: sameAsApproved(fileSvgHtml),
};

/** A changed recording, before over after. */
export const AudioChanged: Story = {
  args: { files: fileAudioChanged },
  play: sameAsApproved(fileAudioChangedHtml),
};

/** A new video. */
export const VideoNew: Story = {
  args: { files: fileVideoNew },
  play: sameAsApproved(fileVideoNewHtml),
};

/** A PDF in a frame, with Open beside it. */
export const Pdf: Story = {
  args: { files: filePdf },
  play: sameAsApproved(filePdfHtml),
};

/** A file past what the surface sends, with Load anyway. */
export const TooLarge: Story = {
  args: { files: fileTooLarge },
  play: sameAsApproved(fileTooLargeHtml),
};

/** A file the surface does not show. */
export const Binary: Story = {
  args: { files: fileBinary },
  play: sameAsApproved(fileBinaryHtml),
};
