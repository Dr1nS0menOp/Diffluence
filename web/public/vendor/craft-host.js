// SPDX-License-Identifier: Apache-2.0
// Ours, not part of PdfCraft or GridCraft. It runs inside the sandboxed frame,
// starts the app, and hands it the one file the page sent, by dropping it on the
// canvas: neither app has another way in. The frame cannot reach the session.
const { entry, canvas } = document.documentElement.dataset;
const START_LIMIT_MS = 5000;

// The app starts after its code has loaded, and a file dropped before it listens is
// lost. So we notice when it starts listening, and then give it two frames.
const listening = new Promise((resolve) => {
  const add = EventTarget.prototype.addEventListener;
  EventTarget.prototype.addEventListener = function addListener(type, ...rest) {
    if (type === 'drop') resolve();
    return add.call(this, type, ...rest);
  };
  setTimeout(resolve, START_LIMIT_MS);
});
const nextFrame = () => new Promise((resolve) => requestAnimationFrame(resolve));

const app = await import(new URL(`./${document.title}/${entry}.js`, new URL('.', import.meta.url)).href);
await app.default({ module_or_path: new URL(`./${document.title}/${entry}_bg.wasm`, new URL('.', import.meta.url)).href });

function open(file) {
  const data = new DataTransfer();
  data.items.add(file);
  const target = document.getElementById(canvas);
  for (const type of ['dragenter', 'dragover', 'drop']) {
    target.dispatchEvent(new DragEvent(type, { dataTransfer: data, bubbles: true, cancelable: true }));
  }
}

// When the app saves a PDF, it hands the browser a download of a blob it made. We keep
// the download, and also send the bytes to the page, which can store them as a new
// version through the normal upload (ARC-099). Only the page decides; this frame stores
// nothing and cannot reach the session.
if (canvas === 'pdfcraft') {
  const blobs = new Map();
  const makeURL = URL.createObjectURL;
  URL.createObjectURL = function createURL(object) {
    const address = makeURL.call(URL, object);
    if (object instanceof Blob) blobs.set(address, object);
    return address;
  };
  const click = HTMLAnchorElement.prototype.click;
  HTMLAnchorElement.prototype.click = function clickAnchor() {
    const blob = this.download ? blobs.get(this.href) : undefined;
    if (blob) {
      blob.arrayBuffer().then((bytes) => window.parent.postMessage({ craft: 'saved', bytes }, '*', [bytes]));
    }
    return click.call(this);
  };
}

window.addEventListener('message', (event) => {
  if (event.source !== window.parent || !(event.data?.bytes instanceof ArrayBuffer)) return;
  open(new File([event.data.bytes], String(event.data.name), { type: String(event.data.type) }));
  window.parent.postMessage({ craft: 'opened' }, '*');
});
window.addEventListener(
  'keydown',
  (event) => {
    if (event.key === 'Escape') window.parent.postMessage({ craft: 'escape' }, '*');
  },
  true,
);
await listening;
await nextFrame();
await nextFrame();
window.parent.postMessage({ craft: 'ready' }, '*');
