import { useEffect, useState } from 'react';
import { API } from '../api';

interface Props {
  onClose: () => void;
}

export default function ChromeModal({ onClose }: Props) {
  const [url, setUrl] = useState('https://www.google.com/chrome/');

  useEffect(() => {
    API.chromeDownloadURL().then(setUrl).catch(() => {});
  }, []);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal chrome" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <h2>Chrome or Brave required</h2>
          <button className="icon-btn" onClick={onClose}>
            ✕
          </button>
        </div>
        <div className="modal-body">
          <p>
            Mellowtel uses your installed Google Chrome, Brave, or Chromium to complete
            rendering jobs and earn. We couldn't find a supported browser on this machine.
          </p>
          <p className="hint">
            Simple fetch-only jobs may still run, but installing Chrome unlocks full earning.
          </p>
        </div>
        <div className="modal-foot">
          <button className="btn ghost" onClick={onClose}>
            Later
          </button>
          <button className="btn primary" onClick={() => API.openURL(url)}>
            Download Chrome ↗
          </button>
        </div>
      </div>
    </div>
  );
}
