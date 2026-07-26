/**
 * Media Providers — the keys AI Studio generates with.
 *
 * Its own page rather than a card inside Settings: this is what you go looking
 * for the moment Studio tells you video or music is unavailable, and it used to
 * be several screens down a page about language models.
 */

import { Clapperboard } from 'lucide-react';
import { Header } from '../components/Header';
import { StudioProviders } from '../components/settings/StudioProviders';

export function MediaProviders() {
  return (
    <div className="flex flex-col h-full">
      <Header title="Media Providers" />
      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="max-w-2xl space-y-4">
          <div className="flex items-start gap-3">
            <div className="w-9 h-9 rounded-lg bg-accent-muted flex items-center justify-center flex-shrink-0">
              <Clapperboard className="w-4 h-4 text-accent-primary" aria-hidden="true" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-text-0">AI Studio providers</h2>
              <p className="text-sm text-text-2 leading-relaxed mt-0.5">
                OpenRouter already covers image generation. Video, music and voice need one of
                these — add any of them and it becomes selectable per generation in Studio. Keys
                are stored encrypted and never sent back to the browser.
              </p>
            </div>
          </div>

          <StudioProviders showHeading={false} />
        </div>
      </div>
    </div>
  );
}
