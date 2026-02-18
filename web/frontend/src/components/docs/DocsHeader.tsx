import { useState, useEffect } from 'react';
import { Link } from 'react-router';
import { Menu, Sun, Moon, Github, Search, LogIn } from 'lucide-react';
import { useDesign } from '../../contexts/DesignContext';

interface DocsHeaderProps {
  scrollContainerId: string;
  onMenuClick: () => void;
}

export function DocsHeader({ scrollContainerId, onMenuClick }: DocsHeaderProps) {
  const [scrolled, setScrolled] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const { mode, accent, updateTheme } = useDesign();

  useEffect(() => {
    const container = document.getElementById(scrollContainerId);
    if (!container) return;

    const handleScroll = () => setScrolled(container.scrollTop > 10);
    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => container.removeEventListener('scroll', handleScroll);
  }, [scrollContainerId]);

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (e.key === 'Escape') setSearchOpen(false);
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, []);

  const toggleMode = () => {
    updateTheme({ accent, mode: mode === 'dark' ? 'light' : 'dark' });
  };

  return (
    <>
      <header
        className={`sticky top-0 z-40 border-b transition-all duration-300 ${
          scrolled
            ? 'bg-surface-1/80 backdrop-blur-xl border-border-0 shadow-sm'
            : 'bg-surface-1 border-transparent'
        }`}
      >
        <div className="flex items-center h-14 px-4 lg:px-6">
          {/* Mobile menu button */}
          <button
            onClick={onMenuClick}
            className="lg:hidden p-2 -ml-2 mr-2 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
            aria-label="Open navigation"
          >
            <Menu className="w-5 h-5" />
          </button>

          {/* Logo */}
          <Link to="/docs" className="flex items-center gap-2.5 shrink-0">
            <div className="w-7 h-7 rounded-lg bg-accent-primary flex items-center justify-center">
              <span className="text-white text-sm font-bold">O</span>
            </div>
            <span className="text-base font-semibold text-text-0">OpenPaw</span>
            <span className="text-xs font-medium px-1.5 py-0.5 rounded bg-accent-muted text-accent-text">
              Docs
            </span>
          </Link>

          {/* Search bar */}
          <div className="flex-1 flex justify-center px-4">
            <button
              onClick={() => setSearchOpen(true)}
              className="hidden sm:flex items-center gap-2 w-full max-w-md px-3 py-1.5 rounded-lg border border-border-0 bg-surface-0/50 text-text-3 text-sm hover:border-border-1 transition-colors cursor-pointer"
            >
              <Search className="w-4 h-4" />
              <span className="flex-1 text-left">Search docs...</span>
              <kbd className="hidden md:inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-surface-2 border border-border-0 text-[10px] font-mono text-text-3">
                <span className="text-xs">&#8984;</span>K
              </kbd>
            </button>
          </div>

          {/* Right actions */}
          <div className="flex items-center gap-1 shrink-0">
            <a
              href="https://github.com/OpenPaw"
              target="_blank"
              rel="noopener noreferrer"
              className="p-2 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors"
              aria-label="GitHub repository"
            >
              <Github className="w-5 h-5" />
            </a>
            <button
              onClick={toggleMode}
              className="p-2 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
              aria-label={mode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
            >
              {mode === 'dark' ? <Sun className="w-5 h-5" /> : <Moon className="w-5 h-5" />}
            </button>
            <Link
              to="/login"
              className="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors"
            >
              <LogIn className="w-4 h-4" />
              Sign In
            </Link>
          </div>
        </div>
      </header>

      {/* Search modal */}
      {searchOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setSearchOpen(false)} />
          <div className="relative w-full max-w-lg mx-4 bg-surface-1 border border-border-0 rounded-xl shadow-2xl overflow-hidden">
            <div className="flex items-center gap-3 px-4 py-3 border-b border-border-0">
              <Search className="w-5 h-5 text-text-3 shrink-0" />
              <input
                type="text"
                placeholder="Search documentation..."
                className="flex-1 bg-transparent text-text-0 text-sm outline-none placeholder:text-text-3"
                autoFocus
              />
              <kbd className="px-1.5 py-0.5 rounded bg-surface-2 border border-border-0 text-[10px] font-mono text-text-3">
                ESC
              </kbd>
            </div>
            <div className="px-4 py-8 text-center text-sm text-text-3">
              Start typing to search...
            </div>
          </div>
        </div>
      )}
    </>
  );
}
