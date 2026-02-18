import { useDesign } from '../contexts/DesignContext';

export function BackgroundImage() {
  const { bgImage } = useDesign();

  if (!bgImage) return null;

  return (
    <>
      <div
        className="fixed inset-0 bg-cover bg-center bg-no-repeat pointer-events-none"
        style={{ backgroundImage: `url(${bgImage})`, zIndex: 0 }}
        aria-hidden="true"
      />
      <div
        className="fixed inset-0 pointer-events-none"
        style={{
          zIndex: 0,
          background: 'linear-gradient(to bottom, rgba(0,0,0,0.75) 0%, rgba(0,0,0,0.85) 50%, rgba(0,0,0,0.92) 100%)',
        }}
        aria-hidden="true"
      />
    </>
  );
}

export function DashboardBackground({ bgImage }: { bgImage: string }) {
  if (!bgImage) return null;

  return (
    <>
      <div
        className="absolute inset-0 bg-cover bg-center bg-no-repeat pointer-events-none"
        style={{ backgroundImage: `url(${bgImage})` }}
        aria-hidden="true"
      />
      <div
        className="absolute inset-0 pointer-events-none"
        style={{
          background: 'linear-gradient(to bottom, rgba(0,0,0,0.7) 0%, rgba(0,0,0,0.82) 50%, rgba(0,0,0,0.9) 100%)',
        }}
        aria-hidden="true"
      />
    </>
  );
}
