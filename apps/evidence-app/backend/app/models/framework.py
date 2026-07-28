from sqlalchemy import String, ForeignKey, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column, relationship
from app.database import Base


class Framework(Base):
    __tablename__ = "frameworks"
    __table_args__ = (UniqueConstraint("product_id", "name", name="uq_framework_product_name"),)

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    product_id: Mapped[int] = mapped_column(
        ForeignKey("products.id", ondelete="CASCADE"),
        nullable=False,
    )
    name: Mapped[str] = mapped_column(String(50), nullable=False)
    description: Mapped[str] = mapped_column(String(500), nullable=True)

    product: Mapped["Product"] = relationship("Product", back_populates="frameworks")
    controls: Mapped[list["Control"]] = relationship(
        "Control",
        back_populates="framework",
        cascade="all, delete-orphan",
    )
