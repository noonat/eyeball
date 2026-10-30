export function Bad({ tone }: { tone: string }) {
  return (
    <div className="pill waiting chrome">
      <span className={`pill ${tone} missing-in-template`} />
      <span className={cx("pill", tone && "missing-in-cx")} />
      <span className="undefined-anywhere" />
    </div>
  );
}
